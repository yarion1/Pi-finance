package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

const (
	emissorTOTP        = "Finanças"
	contextoTOTP       = "dois_fatores.segredo"
	periodoTOTP        = 30
	codigosRecuperacao = 10
)

// IniciarTOTP gera um segredo novo (ainda não confirmado) e devolve a URI
// otpauth:// para o QR code. Substituir um TOTP já ativo exige reautenticação.
func (s *Servico) IniciarTOTP(ctx context.Context, ss *Sessao) (uri, segredo string, err error) {
	if !ss.PodeConfigurar() {
		return "", "", ErrSegundoFator
	}
	if ss.TemTOTP && !ss.Reautenticada(s.agora()) {
		return "", "", ErrReautenticar
	}
	chave, err := totp.Generate(totp.GenerateOpts{
		Issuer:      emissorTOTP,
		AccountName: ss.Email,
		Period:      periodoTOTP,
		Digits:      otp.DigitsSix,
		Algorithm:   otp.AlgorithmSHA1, // o único que todos os aplicativos aceitam
	})
	if err != nil {
		return "", "", err
	}
	cifrado, err := s.Cifrador.Cifrar(chave.Secret(), contextoTOTP)
	if err != nil {
		return "", "", err
	}
	// Um TOTP já ativo continua valendo até o novo ser confirmado.
	_, err = s.Pool.Exec(ctx, `insert into dois_fatores (usuario_id, pendente_cifrado, pendente_expira_em)
		values ($1, $2, $3)
		on conflict (usuario_id) do update set pendente_cifrado = excluded.pendente_cifrado,
			pendente_expira_em = excluded.pendente_expira_em`, ss.UsuarioID, cifrado, s.agora().Add(15*time.Minute))
	if err != nil {
		return "", "", err
	}
	return chave.URL(), chave.Secret(), nil
}

// ConfirmarTOTP ativa o segredo pendente com o primeiro código do aplicativo.
// Se era o primeiro fator do usuário, eleva a sessão e devolve os códigos de
// recuperação (mostrados uma única vez) e o novo token de sessão.
func (s *Servico) ConfirmarTOTP(ctx context.Context, ss *Sessao, codigo string, o Origem) (codigos []string, novoToken string, err error) {
	if !ss.PodeConfigurar() {
		return nil, "", ErrSegundoFator
	}
	err = pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var cifrado string
		err := tx.QueryRow(ctx, `select pendente_cifrado from dois_fatores
			where usuario_id = $1 and pendente_cifrado is not null and pendente_expira_em > $2 for update`,
			ss.UsuarioID, s.agora()).Scan(&cifrado)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNaoPermitido
		}
		if err != nil {
			return err
		}
		segredo, err := s.Cifrador.Decifrar(cifrado, contextoTOTP)
		if err != nil {
			return err
		}
		passo, ok := conferirTOTP(segredo, codigo, 0, s.agora())
		if !ok {
			return ErrCodigo
		}
		if _, err := tx.Exec(ctx, `update dois_fatores set segredo_cifrado = pendente_cifrado, confirmado_em = now(),
			ultimo_passo = $2, pendente_cifrado = null, pendente_expira_em = null where usuario_id = $1`,
			ss.UsuarioID, passo); err != nil {
			return err
		}
		if err := auditar(ctx, tx, ss.UsuarioID, "totp_ativado", "", o, nil); err != nil {
			return err
		}
		if ss.PrecisaConfigurar2FA() {
			if codigos, err = gerarCodigos(ctx, tx, ss.UsuarioID); err != nil {
				return err
			}
			novoToken, err = s.elevar(ctx, tx, ss, "totp_primeira_configuracao", o)
		}
		return err
	})
	return codigos, novoToken, err
}

// conferirTOTP aceita o passo atual e um de cada lado (relógio do celular fora
// de hora), mas nunca um passo já usado: o mesmo código não entra duas vezes.
func conferirTOTP(segredo, codigo string, ultimoPasso int64, agora time.Time) (int64, bool) {
	codigo = strings.ReplaceAll(strings.TrimSpace(codigo), " ", "")
	if len(codigo) != 6 {
		return 0, false
	}
	atual := agora.Unix() / periodoTOTP
	for _, passo := range []int64{atual - 1, atual, atual + 1} {
		if passo <= ultimoPasso {
			continue
		}
		esperado, err := totp.GenerateCodeCustom(segredo, time.Unix(passo*periodoTOTP, 0), totp.ValidateOpts{
			Period: periodoTOTP, Digits: otp.DigitsSix, Algorithm: otp.AlgorithmSHA1,
		})
		if err == nil && subtle.ConstantTimeCompare([]byte(esperado), []byte(codigo)) == 1 {
			return passo, true
		}
	}
	return 0, false
}

func gerarCodigos(ctx context.Context, tx pgx.Tx, usuarioID string) ([]string, error) {
	if _, err := tx.Exec(ctx, "delete from codigos_recuperacao where usuario_id = $1", usuarioID); err != nil {
		return nil, err
	}
	codigos := make([]string, codigosRecuperacao)
	for i := range codigos {
		codigos[i] = novoCodigoRecuperacao()
		if _, err := tx.Exec(ctx, "insert into codigos_recuperacao (usuario_id, codigo_hash) values ($1, $2)",
			usuarioID, HashToken(codigos[i])); err != nil {
			return nil, err
		}
	}
	return codigos, nil
}

// NovosCodigosRecuperacao troca todos os códigos (exige reautenticação).
func (s *Servico) NovosCodigosRecuperacao(ctx context.Context, ss *Sessao, o Origem) ([]string, error) {
	if !ss.MFAOK {
		return nil, ErrSegundoFator
	}
	if !ss.Reautenticada(s.agora()) {
		return nil, ErrReautenticar
	}
	var codigos []string
	err := pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) (err error) {
		if codigos, err = gerarCodigos(ctx, tx, ss.UsuarioID); err != nil {
			return err
		}
		return auditar(ctx, tx, ss.UsuarioID, "codigos_recuperacao_gerados", "", o, nil)
	})
	return codigos, err
}

// VerificarSegundoFator completa o login com o código do aplicativo ou um código
// de recuperação (uso único). Devolve o novo token da sessão.
func (s *Servico) VerificarSegundoFator(ctx context.Context, ss *Sessao, codigo string, o Origem) (string, error) {
	if ss.MFAOK {
		return "", ErrNaoPermitido
	}
	if err := s.bloqueio(ctx, s.Pool, ss.UsuarioID); err != nil {
		return "", err
	}
	var novoToken string
	err := pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		metodo, err := s.conferirSegundoFator(ctx, tx, ss.UsuarioID, codigo)
		if err != nil {
			return err
		}
		novoToken, err = s.elevar(ctx, tx, ss, metodo, o)
		return err
	})
	if errors.Is(err, ErrCodigo) {
		if e := s.registrarFalha(ctx, ss.UsuarioID, "segundo_fator_falhou", o); e != nil {
			return "", e
		}
	}
	return novoToken, err
}

func (s *Servico) conferirSegundoFator(ctx context.Context, tx pgx.Tx, usuarioID, codigo string) (string, error) {
	var (
		cifrado string
		ultimo  int64
	)
	err := tx.QueryRow(ctx, `select segredo_cifrado, ultimo_passo from dois_fatores
		where usuario_id = $1 and confirmado_em is not null for update`, usuarioID).Scan(&cifrado, &ultimo)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if err == nil {
		segredo, err := s.Cifrador.Decifrar(cifrado, contextoTOTP)
		if err != nil {
			return "", err
		}
		if passo, ok := conferirTOTP(segredo, codigo, ultimo, s.agora()); ok {
			_, err := tx.Exec(ctx, "update dois_fatores set ultimo_passo = $2 where usuario_id = $1", usuarioID, passo)
			return "totp", err
		}
	}
	if c := normalizarCodigoRecuperacao(codigo); c != "" {
		tag, err := tx.Exec(ctx, `update codigos_recuperacao set usado_em = now()
			where usuario_id = $1 and codigo_hash = $2 and usado_em is null`, usuarioID, HashToken(c))
		if err != nil {
			return "", err
		}
		if tag.RowsAffected() == 1 {
			return "codigo_recuperacao", nil
		}
	}
	return "", ErrCodigo
}
