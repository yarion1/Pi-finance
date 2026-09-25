package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/jackc/pgx/v5"
)

const validadeDesafio = 5 * time.Minute

// NovoWebAuthn configura a parte confiável a partir da URL pública
// (ex.: https://financas.exemplo.com.br). Devolve nil, sem erro, quando a URL não
// permite passkeys (http na rede de casa ou endereço IP): o login fica só com TOTP.
func NovoWebAuthn(urlPublica string) (*webauthn.WebAuthn, error) {
	u, err := url.Parse(urlPublica)
	if err != nil || u.Host == "" {
		return nil, fmt.Errorf("PUBLIC_URL inválida: %q", urlPublica)
	}
	if !PasskeysPossiveis(urlPublica) {
		return nil, nil
	}
	return webauthn.New(&webauthn.Config{
		RPID:          u.Hostname(),
		RPDisplayName: "Finanças",
		RPOrigins:     []string{u.Scheme + "://" + u.Host},
	})
}

// PasskeysPossiveis: o navegador só oferece passkey em HTTPS (ou localhost) com
// nome de domínio; em http://192.168.x.x não existe.
func PasskeysPossiveis(urlPublica string) bool {
	u, err := url.Parse(urlPublica)
	if err != nil {
		return false
	}
	host := u.Hostname()
	if host == "localhost" {
		return true
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return false
	}
	return u.Scheme == "https"
}

type usuarioWA struct {
	id, email, nome string
	credenciais     []webauthn.Credential
}

func (u *usuarioWA) WebAuthnID() []byte                         { return []byte(u.id) }
func (u *usuarioWA) WebAuthnName() string                       { return u.email }
func (u *usuarioWA) WebAuthnDisplayName() string                { return u.nome }
func (u *usuarioWA) WebAuthnCredentials() []webauthn.Credential { return u.credenciais }

type consultor interface {
	QueryRow(context.Context, string, ...any) pgx.Row
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func carregarUsuarioWA(ctx context.Context, q consultor, usuarioID string) (*usuarioWA, error) {
	u := &usuarioWA{id: usuarioID}
	if err := q.QueryRow(ctx, "select email, nome from usuarios where id = $1", usuarioID).Scan(&u.email, &u.nome); err != nil {
		return nil, err
	}
	linhas, err := q.Query(ctx, "select credencial from passkeys where usuario_id = $1", usuarioID)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	for linhas.Next() {
		var bruto []byte
		if err := linhas.Scan(&bruto); err != nil {
			return nil, err
		}
		var c webauthn.Credential
		if err := json.Unmarshal(bruto, &c); err != nil {
			return nil, err
		}
		u.credenciais = append(u.credenciais, c)
	}
	return u, linhas.Err()
}

func (s *Servico) guardarDesafio(ctx context.Context, usuarioID *string, dados *webauthn.SessionData) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx, "insert into desafios_webauthn (usuario_id, dados, expira_em) values ($1, $2, $3) returning id",
		usuarioID, dados, s.agora().Add(validadeDesafio)).Scan(&id)
	return id, err
}

// consumirDesafio devolve os dados do desafio e o apaga (uso único).
func consumirDesafio(ctx context.Context, tx pgx.Tx, id string, agora time.Time) (webauthn.SessionData, *string, error) {
	var (
		dados     webauthn.SessionData
		usuarioID *string
		expira    time.Time
	)
	err := tx.QueryRow(ctx, "delete from desafios_webauthn where id = $1 returning dados, usuario_id, expira_em", id).
		Scan(&dados, &usuarioID, &expira)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !expira.After(agora)) {
		return dados, nil, ErrPasskey
	}
	return dados, usuarioID, err
}

// IniciarRegistroPasskey devolve as opções para navigator.credentials.create().
func (s *Servico) IniciarRegistroPasskey(ctx context.Context, ss *Sessao) (*protocol.CredentialCreation, string, error) {
	if s.WebAuthn == nil {
		return nil, "", ErrPasskeyIndisponivel
	}
	if !ss.PodeConfigurar() {
		return nil, "", ErrSegundoFator
	}
	u, err := carregarUsuarioWA(ctx, s.Pool, ss.UsuarioID)
	if err != nil {
		return nil, "", err
	}
	excluir := make([]protocol.CredentialDescriptor, 0, len(u.credenciais))
	for _, c := range u.credenciais {
		excluir = append(excluir, c.Descriptor())
	}
	opcoes, dados, err := s.WebAuthn.BeginRegistration(u,
		webauthn.WithResidentKeyRequirement(protocol.ResidentKeyRequirementRequired),
		webauthn.WithExclusions(excluir),
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
			UserVerification: protocol.VerificationRequired,
		}),
	)
	if err != nil {
		return nil, "", err
	}
	id, err := s.guardarDesafio(ctx, &ss.UsuarioID, dados)
	return opcoes, id, err
}

// ConcluirRegistroPasskey grava a passkey. Se era o primeiro fator do usuário,
// eleva a sessão e devolve os códigos de recuperação e o novo token.
func (s *Servico) ConcluirRegistroPasskey(ctx context.Context, ss *Sessao, desafioID string, resposta []byte, nome string, o Origem) (codigos []string, novoToken string, err error) {
	if s.WebAuthn == nil {
		return nil, "", ErrPasskeyIndisponivel
	}
	if !ss.PodeConfigurar() {
		return nil, "", ErrSegundoFator
	}
	nome = strings.TrimSpace(nome)
	if nome == "" {
		nome = "Passkey"
	}
	if len(nome) > 60 {
		nome = nome[:60]
	}
	analisada, err := protocol.ParseCredentialCreationResponseBytes(resposta)
	if err != nil {
		return nil, "", ErrPasskey
	}
	err = pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		dados, dono, err := consumirDesafio(ctx, tx, desafioID, s.agora())
		if err != nil {
			return err
		}
		if dono == nil || *dono != ss.UsuarioID {
			return ErrPasskey
		}
		u, err := carregarUsuarioWA(ctx, tx, ss.UsuarioID)
		if err != nil {
			return err
		}
		cred, err := s.WebAuthn.CreateCredential(u, dados, analisada)
		if err != nil {
			return ErrPasskey
		}
		if _, err := tx.Exec(ctx, "insert into passkeys (usuario_id, credencial_id, credencial, nome) values ($1, $2, $3, $4)",
			ss.UsuarioID, cred.ID, cred, nome); err != nil {
			return err
		}
		if err := auditar(ctx, tx, ss.UsuarioID, "passkey_adicionada", nome, o, nil); err != nil {
			return err
		}
		if ss.PrecisaConfigurar2FA() {
			if codigos, err = gerarCodigos(ctx, tx, ss.UsuarioID); err != nil {
				return err
			}
			novoToken, err = s.elevar(ctx, tx, ss, "passkey_primeira_configuracao", o)
		}
		return err
	})
	return codigos, novoToken, err
}

// IniciarLoginPasskey: sem sessão, login direto (passkey descoberta pelo
// navegador); com sessão parcial, a passkey é o segundo fator daquele usuário.
func (s *Servico) IniciarLoginPasskey(ctx context.Context, parcial *Sessao) (*protocol.CredentialAssertion, string, error) {
	if s.WebAuthn == nil {
		return nil, "", ErrPasskeyIndisponivel
	}
	if parcial != nil && !parcial.MFAOK {
		u, err := carregarUsuarioWA(ctx, s.Pool, parcial.UsuarioID)
		if err != nil {
			return nil, "", err
		}
		if len(u.credenciais) == 0 {
			return nil, "", ErrNaoPermitido
		}
		opcoes, dados, err := s.WebAuthn.BeginLogin(u, webauthn.WithUserVerification(protocol.VerificationRequired))
		if err != nil {
			return nil, "", err
		}
		id, err := s.guardarDesafio(ctx, &parcial.UsuarioID, dados)
		return opcoes, id, err
	}
	opcoes, dados, err := s.WebAuthn.BeginDiscoverableLogin(webauthn.WithUserVerification(protocol.VerificationRequired))
	if err != nil {
		return nil, "", err
	}
	id, err := s.guardarDesafio(ctx, nil, dados)
	return opcoes, id, err
}

// ConcluirLoginPasskey valida a asserção e devolve o token da sessão completa
// (nova, ou a parcial elevada).
func (s *Servico) ConcluirLoginPasskey(ctx context.Context, parcial *Sessao, desafioID string, resposta []byte, o Origem) (string, error) {
	if s.WebAuthn == nil {
		return "", ErrPasskeyIndisponivel
	}
	analisada, err := protocol.ParseCredentialRequestResponseBytes(resposta)
	if err != nil {
		return "", ErrPasskey
	}
	var token string
	err = pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		dados, dono, err := consumirDesafio(ctx, tx, desafioID, s.agora())
		if err != nil {
			return err
		}
		var (
			u    *usuarioWA
			cred *webauthn.Credential
		)
		if dono != nil {
			if parcial == nil || parcial.UsuarioID != *dono {
				return ErrPasskey
			}
			if u, err = carregarUsuarioWA(ctx, tx, *dono); err != nil {
				return err
			}
			if cred, err = s.WebAuthn.ValidateLogin(u, dados, analisada); err != nil {
				return ErrPasskey
			}
		} else {
			achar := func(_, handle []byte) (webauthn.User, error) {
				var e error
				u, e = carregarUsuarioWA(ctx, tx, string(handle))
				return u, e
			}
			if _, cred, err = s.WebAuthn.ValidatePasskeyLogin(achar, dados, analisada); err != nil {
				return ErrPasskey
			}
		}
		if err := s.bloqueio(ctx, tx, u.id); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "update passkeys set credencial = $2, usada_em = now() where credencial_id = $1 and usuario_id = $3",
			cred.ID, cred, u.id); err != nil {
			return err
		}
		sessao := parcial
		if sessao == nil || sessao.MFAOK || sessao.UsuarioID != u.id {
			id, err := s.criarSessaoParcialID(ctx, tx, u.id, o)
			if err != nil {
				return err
			}
			sessao = &Sessao{ID: id, UsuarioID: u.id}
		}
		token, err = s.elevar(ctx, tx, sessao, "passkey", o)
		return err
	})
	return token, err
}

func (s *Servico) criarSessaoParcialID(ctx context.Context, tx pgx.Tx, usuarioID string, o Origem) (string, error) {
	var id string
	err := tx.QueryRow(ctx, `insert into sessoes (usuario_id, token_hash, mfa_ok, expira_em, ip, user_agent)
		values ($1, $2, false, $3, $4, $5) returning id`,
		usuarioID, HashToken(NovoToken()), s.agora().Add(DuracaoParcial), o.IP, truncar(o.UserAgent, 300)).Scan(&id)
	return id, err
}

// Passkey é o que a tela de segurança lista.
type Passkey struct {
	ID       string     `json:"id"`
	Nome     string     `json:"nome"`
	CriadaEm time.Time  `json:"criada_em"`
	UsadaEm  *time.Time `json:"usada_em"`
}

// ListarPasskeys do usuário da sessão.
func (s *Servico) ListarPasskeys(ctx context.Context, ss *Sessao) ([]Passkey, error) {
	linhas, err := s.Pool.Query(ctx, "select id, nome, criada_em, usada_em from passkeys where usuario_id = $1 order by criada_em", ss.UsuarioID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(linhas, pgx.RowToStructByPos[Passkey])
}

// RemoverPasskey exige reautenticação e que sobre pelo menos um segundo fator.
func (s *Servico) RemoverPasskey(ctx context.Context, ss *Sessao, id string, o Origem) error {
	if !ss.MFAOK {
		return ErrSegundoFator
	}
	if !ss.Reautenticada(s.agora()) {
		return ErrReautenticar
	}
	if !ss.TemTOTP && ss.Passkeys <= 1 {
		return ErrUltimoFator
	}
	return pgx.BeginTxFunc(ctx, s.Pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		var nome string
		err := tx.QueryRow(ctx, "delete from passkeys where id = $1 and usuario_id = $2 returning nome", id, ss.UsuarioID).Scan(&nome)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNaoPermitido
		}
		if err != nil {
			return err
		}
		return auditar(ctx, tx, ss.UsuarioID, "passkey_removida", nome, o, nil)
	})
}
