// Package auth cuida de cadastro, login, sessões e segundo fator (TOTP e passkeys).
//
// Modelo de sessão:
//   - senha correta cria uma sessão parcial (mfa_ok = false) de 30 minutos;
//   - a sessão parcial só serve para confirmar o segundo fator ou, se o usuário
//     ainda não tem nenhum, para configurar o primeiro;
//   - confirmado o segundo fator, o token é trocado e a sessão passa a valer
//     7 dias no celular ou 12 horas no navegador (sem renovação automática);
//   - login só com passkey já nasce completo (a passkey é multifator).
package auth

import (
	"context"
	"errors"
	"log/slog"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yarion1/pi-finance/internal/cripto"
)

const (
	DuracaoParcial     = 30 * time.Minute
	DuracaoCelular     = 7 * 24 * time.Hour
	DuracaoNavegador   = 12 * time.Hour
	JanelaReautenticar = 10 * time.Minute
	ValidadeConvite    = 48 * time.Hour
	tentativasLivres   = 5
	bloqueioBase       = 15 * time.Minute
	bloqueioMaximo     = 24 * time.Hour
)

// Servico concentra as regras de autenticação.
type Servico struct {
	Pool     *pgxpool.Pool
	Cifrador *cripto.Cifrador
	WebAuthn *webauthn.WebAuthn
	Agora    func() time.Time
}

func (s *Servico) agora() time.Time {
	if s.Agora != nil {
		return s.Agora()
	}
	return time.Now()
}

// Sessao é a sessão de uma requisição autenticada.
type Sessao struct {
	ID              string
	UsuarioID       string
	Nome            string
	Email           string
	MFAOK           bool
	ExpiraEm        time.Time
	ReautenticadaEm *time.Time
	TemTOTP         bool
	Passkeys        int
}

// PrecisaConfigurar2FA: o usuário ainda não tem nenhum segundo fator.
func (s *Sessao) PrecisaConfigurar2FA() bool { return !s.TemTOTP && s.Passkeys == 0 }

// PodeConfigurar: sessão completa ou usuário sem nenhum fator (primeira configuração).
func (s *Sessao) PodeConfigurar() bool { return s.MFAOK || s.PrecisaConfigurar2FA() }

// Reautenticada: senha confirmada há pouco (para ações sensíveis).
func (s *Sessao) Reautenticada(agora time.Time) bool {
	return s.ReautenticadaEm != nil && agora.Sub(*s.ReautenticadaEm) <= JanelaReautenticar
}

// Contexto da requisição, gravado na sessão e na auditoria.
type Origem struct {
	IP        string
	UserAgent string
}

var reCelular = regexp.MustCompile(`(?i)android|iphone|ipad|mobile`)

// EhCelular decide a duração da sessão (7 dias no celular, 12 h no navegador).
func EhCelular(userAgent string) bool { return reCelular.MatchString(userAgent) }

func duracaoSessao(userAgent string) time.Duration {
	if EhCelular(userAgent) {
		return DuracaoCelular
	}
	return DuracaoNavegador
}

// DadosCadastro são os campos do formulário de cadastro.
type DadosCadastro struct {
	Nome, Email, Senha, Convite string
}

func normalizarEmail(e string) (string, error) {
	e = strings.ToLower(strings.TrimSpace(e))
	end, err := mail.ParseAddress(e)
	if err != nil || end.Address != e {
		return "", ErrDadosInvalidos
	}
	return e, nil
}

func validarSenha(senha string) error {
	n := len([]rune(senha))
	if n < SenhaTamMinimo || n > SenhaTamMaximo {
		return ErrDadosInvalidos
	}
	return nil
}

// Cadastrar cria o usuário e já devolve uma sessão parcial (ele vai direto
// configurar o segundo fator). O primeiro usuário do sistema entra sem convite;
// os demais só com convite válido.
func (s *Servico) Cadastrar(ctx context.Context, d DadosCadastro, o Origem) (token string, err error) {
	nome := strings.TrimSpace(d.Nome)
	email, err := normalizarEmail(d.Email)
	if err != nil || nome == "" || len(nome) > 100 {
		return "", ErrDadosInvalidos
	}
	if err := validarSenha(d.Senha); err != nil {
		return "", err
	}
	hash, err := HashSenha(d.Senha)
	if err != nil {
		return "", err
	}

	err = pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		// serializa cadastros para a regra do "primeiro usuário" não ter corrida
		if _, err := tx.Exec(ctx, "select pg_advisory_xact_lock(7201001)"); err != nil {
			return err
		}
		var existeAlguem bool
		if err := tx.QueryRow(ctx, "select exists (select 1 from usuarios)").Scan(&existeAlguem); err != nil {
			return err
		}
		var tokenConvite []byte
		if d.Convite != "" {
			tokenConvite = HashToken(d.Convite)
			var valido bool
			err := tx.QueryRow(ctx, "select valido from consultar_convite($1)", tokenConvite).Scan(&valido)
			if errors.Is(err, pgx.ErrNoRows) || (err == nil && !valido) {
				return ErrConviteInvalido
			}
			if err != nil {
				return err
			}
		} else if existeAlguem {
			return ErrCadastroFechado
		}

		var usuarioID string
		err := tx.QueryRow(ctx,
			"insert into usuarios (nome, email, senha_hash) values ($1, $2, $3) returning id",
			nome, email, hash).Scan(&usuarioID)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return ErrEmailEmUso
		}
		if err != nil {
			return err
		}
		if tokenConvite != nil {
			if _, err := tx.Exec(ctx, "select set_config('app.usuario_id', $1, true)", usuarioID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, "select aceitar_convite($1)", tokenConvite); err != nil {
				return ErrConviteInvalido
			}
		}
		token, err = s.criarSessao(ctx, tx, usuarioID, false, o)
		if err != nil {
			return err
		}
		return auditar(ctx, tx, usuarioID, "cadastro", "", o, map[string]any{"com_convite": tokenConvite != nil})
	})
	return token, err
}

func (s *Servico) criarSessao(ctx context.Context, tx pgx.Tx, usuarioID string, mfaOK bool, o Origem) (string, error) {
	token := NovoToken()
	duracao := DuracaoParcial
	if mfaOK {
		duracao = duracaoSessao(o.UserAgent)
	}
	_, err := tx.Exec(ctx, `insert into sessoes (usuario_id, token_hash, mfa_ok, expira_em, ip, user_agent)
		values ($1, $2, $3, $4, $5, $6)`,
		usuarioID, HashToken(token), mfaOK, s.agora().Add(duracao), o.IP, truncar(o.UserAgent, 300))
	return token, err
}

func truncar(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// Entrar confere e-mail e senha e cria uma sessão parcial.
func (s *Servico) Entrar(ctx context.Context, email, senha string, o Origem) (string, error) {
	email, err := normalizarEmail(email)
	if err != nil {
		_, _ = ConferirSenha(senha, hashFalso)
		return "", ErrCredenciais
	}
	var (
		usuarioID, hash string
		bloqueadoAte    *time.Time
	)
	err = s.Pool.QueryRow(ctx, "select id, senha_hash, bloqueado_ate from usuarios where lower(email) = $1", email).
		Scan(&usuarioID, &hash, &bloqueadoAte)
	if errors.Is(err, pgx.ErrNoRows) {
		_, _ = ConferirSenha(senha, hashFalso)
		return "", ErrCredenciais
	}
	if err != nil {
		return "", err
	}
	if bloqueadoAte != nil && bloqueadoAte.After(s.agora()) {
		return "", ErrBloqueado{Ate: *bloqueadoAte}
	}
	ok, err := ConferirSenha(senha, hash)
	if err != nil {
		return "", err
	}
	if !ok {
		if err := s.registrarFalha(ctx, usuarioID, "login_falhou", o); err != nil {
			return "", err
		}
		return "", ErrCredenciais
	}

	var token string
	err = pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "update usuarios set tentativas_falhas = 0, bloqueado_ate = null where id = $1", usuarioID); err != nil {
			return err
		}
		token, err = s.criarSessao(ctx, tx, usuarioID, false, o)
		if err != nil {
			return err
		}
		return auditar(ctx, tx, usuarioID, "senha_ok", "", o, nil)
	})
	return token, err
}

// registrarFalha conta a tentativa errada e bloqueia a partir da 5ª, dobrando o
// tempo a cada nova falha (15 min, 30 min, 1 h... até 24 h).
func (s *Servico) registrarFalha(ctx context.Context, usuarioID, acao string, o Origem) error {
	return pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var falhas int
		if err := tx.QueryRow(ctx,
			"update usuarios set tentativas_falhas = tentativas_falhas + 1 where id = $1 returning tentativas_falhas",
			usuarioID).Scan(&falhas); err != nil {
			return err
		}
		dados := map[string]any{"tentativas": falhas}
		if falhas >= tentativasLivres {
			ate := s.agora().Add(TempoBloqueio(falhas))
			if _, err := tx.Exec(ctx, "update usuarios set bloqueado_ate = $2 where id = $1", usuarioID, ate); err != nil {
				return err
			}
			dados["bloqueado_ate"] = ate
		}
		return auditar(ctx, tx, usuarioID, acao, "", o, dados)
	})
}

// TempoBloqueio para a n-ésima falha seguida (n >= 5).
func TempoBloqueio(falhas int) time.Duration {
	if falhas < tentativasLivres {
		return 0
	}
	d := bloqueioBase
	for i := tentativasLivres; i < falhas; i++ {
		d *= 2
		if d >= bloqueioMaximo {
			return bloqueioMaximo
		}
	}
	return d
}

func (s *Servico) bloqueio(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, usuarioID string) error {
	var ate *time.Time
	if err := q.QueryRow(ctx, "select bloqueado_ate from usuarios where id = $1", usuarioID).Scan(&ate); err != nil {
		return err
	}
	if ate != nil && ate.After(s.agora()) {
		return ErrBloqueado{Ate: *ate}
	}
	return nil
}

// SessaoPorToken valida o cookie. Sessões expiradas são apagadas.
func (s *Servico) SessaoPorToken(ctx context.Context, token string) (*Sessao, error) {
	if token == "" {
		return nil, ErrSessao
	}
	var ss Sessao
	err := s.Pool.QueryRow(ctx, `
		select s.id, s.usuario_id, u.nome, u.email, s.mfa_ok, s.expira_em, s.reautenticada_em,
		       exists (select 1 from dois_fatores f where f.usuario_id = u.id and f.segredo_cifrado is not null),
		       (select count(*) from passkeys p where p.usuario_id = u.id)
		from sessoes s join usuarios u on u.id = s.usuario_id
		where s.token_hash = $1`, HashToken(token)).
		Scan(&ss.ID, &ss.UsuarioID, &ss.Nome, &ss.Email, &ss.MFAOK, &ss.ExpiraEm, &ss.ReautenticadaEm, &ss.TemTOTP, &ss.Passkeys)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSessao
	}
	if err != nil {
		return nil, err
	}
	if !ss.ExpiraEm.After(s.agora()) {
		_, _ = s.Pool.Exec(ctx, "delete from sessoes where id = $1", ss.ID)
		return nil, ErrSessao
	}
	return &ss, nil
}

// Sair apaga a sessão.
func (s *Servico) Sair(ctx context.Context, ss *Sessao, o Origem) error {
	return pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "delete from sessoes where id = $1", ss.ID); err != nil {
			return err
		}
		return auditar(ctx, tx, ss.UsuarioID, "logout", "", o, nil)
	})
}

// elevar marca a sessão como completa, trocando o token (evita fixação de sessão).
// Na primeira vez em que o usuário completa o login num aparelho, gera um alerta.
func (s *Servico) elevar(ctx context.Context, tx pgx.Tx, ss *Sessao, metodo string, o Origem) (string, error) {
	token := NovoToken()
	_, err := tx.Exec(ctx, `update sessoes set token_hash = $2, mfa_ok = true, expira_em = $3, ip = $4, user_agent = $5
		where id = $1`, ss.ID, HashToken(token), s.agora().Add(duracaoSessao(o.UserAgent)), o.IP, truncar(o.UserAgent, 300))
	if err != nil {
		return "", err
	}
	if _, err := tx.Exec(ctx, "update usuarios set tentativas_falhas = 0, bloqueado_ate = null where id = $1", ss.UsuarioID); err != nil {
		return "", err
	}
	if err := s.avisarAparelhoNovo(ctx, tx, ss.UsuarioID, ss.ID, o); err != nil {
		return "", err
	}
	return token, auditar(ctx, tx, ss.UsuarioID, "login", "", o, map[string]any{"metodo": metodo})
}

// avisarAparelhoNovo cria um alerta quando o navegador nunca completou login antes.
// O envio por Telegram e e-mail entra junto com as notificações (fase 7).
func (s *Servico) avisarAparelhoNovo(ctx context.Context, tx pgx.Tx, usuarioID, sessaoAtual string, o Origem) error {
	// auditoria tem RLS: o usuário precisa estar na transação para ler o próprio histórico
	if _, err := tx.Exec(ctx, "select set_config('app.usuario_id', $1, true)", usuarioID); err != nil {
		return err
	}
	var conhecido, primeiro bool
	err := tx.QueryRow(ctx, `select
		exists (select 1 from auditoria where usuario_id = $1 and acao = 'login' and dados->>'user_agent' = $2),
		not exists (select 1 from auditoria where usuario_id = $1 and acao = 'login')`,
		usuarioID, truncar(o.UserAgent, 300)).Scan(&conhecido, &primeiro)
	if err != nil || conhecido || primeiro {
		return err
	}
	_, err = tx.Exec(ctx, `insert into alertas (usuario_id, tipo, dados) values ($1, 'login_aparelho_novo', $2)`,
		usuarioID, map[string]any{"ip": o.IP, "user_agent": truncar(o.UserAgent, 300), "sessao_id": sessaoAtual})
	return err
}

// Reautenticar confirma a senha de novo para liberar ações sensíveis por 10 minutos.
func (s *Servico) Reautenticar(ctx context.Context, ss *Sessao, senha string, o Origem) error {
	if !ss.MFAOK {
		return ErrSegundoFator
	}
	if err := s.bloqueio(ctx, s.Pool, ss.UsuarioID); err != nil {
		return err
	}
	var hash string
	if err := s.Pool.QueryRow(ctx, "select senha_hash from usuarios where id = $1", ss.UsuarioID).Scan(&hash); err != nil {
		return err
	}
	ok, err := ConferirSenha(senha, hash)
	if err != nil {
		return err
	}
	if !ok {
		if err := s.registrarFalha(ctx, ss.UsuarioID, "reautenticacao_falhou", o); err != nil {
			return err
		}
		return ErrCredenciais
	}
	_, err = s.Pool.Exec(ctx, "update sessoes set reautenticada_em = now() where id = $1", ss.ID)
	return err
}

// Limpar apaga sessões e desafios vencidos (roda no worker).
func Limpar(ctx context.Context, pool *pgxpool.Pool) error {
	for _, q := range []string{
		"delete from sessoes where expira_em < now()",
		"delete from desafios_webauthn where expira_em < now()",
		"delete from convites_casa where aceito_em is null and expira_em < now() - interval '7 days'",
	} {
		tag, err := pool.Exec(ctx, q)
		if err != nil {
			return err
		}
		if tag.RowsAffected() > 0 {
			slog.Info("limpeza", "consulta", q, "linhas", tag.RowsAffected())
		}
	}
	return nil
}

func auditar(ctx context.Context, tx pgx.Tx, usuarioID, acao, alvo string, o Origem, dados map[string]any) error {
	if dados == nil {
		dados = map[string]any{}
	}
	dados["user_agent"] = truncar(o.UserAgent, 300)
	var uid any
	if usuarioID != "" {
		uid = usuarioID
	}
	_, err := tx.Exec(ctx, "insert into auditoria (usuario_id, acao, alvo, ip, dados) values ($1, $2, nullif($3, ''), $4, $5)",
		uid, acao, alvo, o.IP, dados)
	return err
}

// Auditar grava um evento fora do fluxo de login (troca de permissão, exclusão...).
func Auditar(ctx context.Context, tx pgx.Tx, usuarioID, acao, alvo string, o Origem, dados map[string]any) error {
	return auditar(ctx, tx, usuarioID, acao, alvo, o, dados)
}
