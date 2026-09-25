package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/yarion1/pi-finance/internal/auth"
)

type respostaSessao struct {
	Autenticado       bool   `json:"autenticado"`
	UsuarioID         string `json:"usuario_id,omitempty"`
	Nome              string `json:"nome,omitempty"`
	Email             string `json:"email,omitempty"`
	MFAOK             bool   `json:"mfa_ok"`
	TemTOTP           bool   `json:"tem_totp"`
	Passkeys          int    `json:"passkeys"`
	PrecisaConfigurar bool   `json:"precisa_configurar_2fa"`
	Reautenticada     bool   `json:"reautenticada"`
}

func (s *Servidor) estadoCadastro(w http.ResponseWriter, r *http.Request) {
	var existe bool
	if err := s.Pool.QueryRow(r.Context(), "select exists (select 1 from usuarios)").Scan(&existe); err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, map[string]bool{"cadastro_aberto": !existe})
}

func (s *Servidor) verSessao(w http.ResponseWriter, r *http.Request) {
	ss := sessaoDe(r)
	if ss == nil {
		escreverJSON(w, http.StatusOK, respostaSessao{})
		return
	}
	escreverJSON(w, http.StatusOK, respostaSessao{
		Autenticado: true, UsuarioID: ss.UsuarioID, Nome: ss.Nome, Email: ss.Email, MFAOK: ss.MFAOK,
		TemTOTP: ss.TemTOTP, Passkeys: ss.Passkeys, PrecisaConfigurar: ss.PrecisaConfigurar2FA(),
		Reautenticada: ss.Reautenticada(time.Now()),
	})
}

func (s *Servidor) cadastrar(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Nome    string `json:"nome"`
		Email   string `json:"email"`
		Senha   string `json:"senha"`
		Convite string `json:"convite"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	token, err := s.Auth.Cadastrar(r.Context(), auth.DadosCadastro{Nome: c.Nome, Email: c.Email, Senha: c.Senha, Convite: c.Convite}, s.origem(r))
	if err != nil {
		falhar(w, r, err)
		return
	}
	s.gravarSessao(w, token, false, r.UserAgent())
	escreverJSON(w, http.StatusCreated, map[string]bool{"ok": true})
}

func (s *Servidor) entrar(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Email string `json:"email"`
		Senha string `json:"senha"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	token, err := s.Auth.Entrar(r.Context(), c.Email, c.Senha, s.origem(r))
	if err != nil {
		falhar(w, r, err)
		return
	}
	s.gravarSessao(w, token, false, r.UserAgent())
	escreverJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Servidor) sair(w http.ResponseWriter, r *http.Request) {
	if err := s.Auth.Sair(r.Context(), sessaoDe(r), s.origem(r)); err != nil {
		falhar(w, r, err)
		return
	}
	s.apagarCookie(w, cookieSessao)
	w.WriteHeader(http.StatusNoContent)
}

type corpoCodigo struct {
	Codigo string `json:"codigo"`
}

func (s *Servidor) verificar2FA(w http.ResponseWriter, r *http.Request) {
	var c corpoCodigo
	if !lerJSON(w, r, &c) {
		return
	}
	token, err := s.Auth.VerificarSegundoFator(r.Context(), sessaoDe(r), c.Codigo, s.origem(r))
	if err != nil {
		falhar(w, r, err)
		return
	}
	s.gravarSessao(w, token, true, r.UserAgent())
	escreverJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Servidor) reautenticar(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Senha string `json:"senha"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	if err := s.Auth.Reautenticar(r.Context(), sessaoDe(r), c.Senha, s.origem(r)); err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) iniciarTOTP(w http.ResponseWriter, r *http.Request) {
	uri, segredo, err := s.Auth.IniciarTOTP(r.Context(), sessaoDe(r))
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, map[string]string{"uri": uri, "segredo": segredo})
}

// respostaFator: códigos de recuperação só vêm na primeira configuração.
type respostaFator struct {
	Codigos []string `json:"codigos_recuperacao,omitempty"`
}

func (s *Servidor) confirmarTOTP(w http.ResponseWriter, r *http.Request) {
	var c corpoCodigo
	if !lerJSON(w, r, &c) {
		return
	}
	codigos, token, err := s.Auth.ConfirmarTOTP(r.Context(), sessaoDe(r), c.Codigo, s.origem(r))
	if err != nil {
		falhar(w, r, err)
		return
	}
	if token != "" {
		s.gravarSessao(w, token, true, r.UserAgent())
	}
	escreverJSON(w, http.StatusOK, respostaFator{Codigos: codigos})
}

func (s *Servidor) gerarRecuperacao(w http.ResponseWriter, r *http.Request) {
	codigos, err := s.Auth.NovosCodigosRecuperacao(r.Context(), sessaoDe(r), s.origem(r))
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, respostaFator{Codigos: codigos})
}

func (s *Servidor) iniciarRegistroPasskey(w http.ResponseWriter, r *http.Request) {
	opcoes, desafio, err := s.Auth.IniciarRegistroPasskey(r.Context(), sessaoDe(r))
	if err != nil {
		falhar(w, r, err)
		return
	}
	s.gravarCookie(w, cookieDesafio, desafio, 5*time.Minute)
	escreverJSON(w, http.StatusOK, opcoes)
}

func (s *Servidor) concluirRegistroPasskey(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Nome       string          `json:"nome"`
		Credencial json.RawMessage `json:"credencial"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	desafio, err := r.Cookie(cookieDesafio)
	if err != nil {
		falhar(w, r, auth.ErrPasskey)
		return
	}
	s.apagarCookie(w, cookieDesafio)
	codigos, token, err := s.Auth.ConcluirRegistroPasskey(r.Context(), sessaoDe(r), desafio.Value, c.Credencial, c.Nome, s.origem(r))
	if err != nil {
		falhar(w, r, err)
		return
	}
	if token != "" {
		s.gravarSessao(w, token, true, r.UserAgent())
	}
	escreverJSON(w, http.StatusOK, respostaFator{Codigos: codigos})
}

func (s *Servidor) iniciarLoginPasskey(w http.ResponseWriter, r *http.Request) {
	opcoes, desafio, err := s.Auth.IniciarLoginPasskey(r.Context(), sessaoDe(r))
	if err != nil {
		falhar(w, r, err)
		return
	}
	s.gravarCookie(w, cookieDesafio, desafio, 5*time.Minute)
	escreverJSON(w, http.StatusOK, opcoes)
}

func (s *Servidor) concluirLoginPasskey(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Credencial json.RawMessage `json:"credencial"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	desafio, err := r.Cookie(cookieDesafio)
	if err != nil {
		falhar(w, r, auth.ErrPasskey)
		return
	}
	s.apagarCookie(w, cookieDesafio)
	token, err := s.Auth.ConcluirLoginPasskey(r.Context(), sessaoDe(r), desafio.Value, c.Credencial, s.origem(r))
	if err != nil {
		falhar(w, r, err)
		return
	}
	s.gravarSessao(w, token, true, r.UserAgent())
	escreverJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Servidor) listarPasskeys(w http.ResponseWriter, r *http.Request) {
	lista, err := s.Auth.ListarPasskeys(r.Context(), sessaoDe(r))
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

func (s *Servidor) removerPasskey(w http.ResponseWriter, r *http.Request) {
	if err := s.Auth.RemoverPasskey(r.Context(), sessaoDe(r), r.PathValue("id"), s.origem(r)); err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
