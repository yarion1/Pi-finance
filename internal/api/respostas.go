package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/yarion1/pi-finance/internal/auth"
	"github.com/yarion1/pi-finance/internal/financeiro"
)

func escreverJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func erroJSON(w http.ResponseWriter, status int, codigo, mensagem string) {
	escreverJSON(w, status, map[string]string{"erro": codigo, "mensagem": mensagem})
}

// lerJSON decodifica o corpo, recusando campos desconhecidos.
func lerJSON(w http.ResponseWriter, r *http.Request, destino any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(destino); err != nil {
		erroJSON(w, http.StatusBadRequest, "json", "corpo inválido")
		return false
	}
	return true
}

// falhar traduz erros conhecidos em respostas; o resto vira 500 com log.
func falhar(w http.ResponseWriter, r *http.Request, err error) {
	var bloq auth.ErrBloqueado
	var pgErr *pgconn.PgError
	switch {
	case errors.As(err, &bloq):
		erroJSON(w, http.StatusTooManyRequests, "bloqueado", bloq.Error())
	case errors.Is(err, auth.ErrCredenciais), errors.Is(err, auth.ErrCodigo), errors.Is(err, auth.ErrPasskey):
		erroJSON(w, http.StatusUnauthorized, "credenciais", err.Error())
	case errors.Is(err, auth.ErrSessao):
		erroJSON(w, http.StatusUnauthorized, "sessao", err.Error())
	case errors.Is(err, auth.ErrSegundoFator):
		erroJSON(w, http.StatusForbidden, "segundo_fator", err.Error())
	case errors.Is(err, auth.ErrReautenticar):
		erroJSON(w, http.StatusForbidden, "reautenticar", err.Error())
	case errors.Is(err, auth.ErrNaoPermitido), errors.Is(err, auth.ErrCadastroFechado):
		erroJSON(w, http.StatusForbidden, "proibido", err.Error())
	case errors.Is(err, auth.ErrConviteInvalido):
		erroJSON(w, http.StatusGone, "convite", err.Error())
	case errors.Is(err, auth.ErrEmailEmUso):
		erroJSON(w, http.StatusConflict, "email", err.Error())
	case errors.Is(err, financeiro.ErrMoeda):
		erroJSON(w, http.StatusBadRequest, "validacao", err.Error())
	case errors.Is(err, auth.ErrDadosInvalidos), errors.Is(err, auth.ErrUltimoFator), errors.Is(err, auth.ErrPasskeyIndisponivel),
		errors.Is(err, errValidacao):
		erroJSON(w, http.StatusBadRequest, "validacao", mensagemValidacao(err))
	case errors.Is(err, pgx.ErrNoRows), errors.Is(err, errNaoEncontrado), errors.Is(err, financeiro.ErrContaNaoEditavel):
		erroJSON(w, http.StatusNotFound, "nao_encontrado", "não encontrado")
	case errors.As(err, &pgErr) && pgErr.Code == "42501":
		erroJSON(w, http.StatusForbidden, "proibido", "sem permissão")
	case errors.As(err, &pgErr) && (pgErr.Code == "23514" || pgErr.Code == "P0002"):
		erroJSON(w, http.StatusBadRequest, "validacao", pgErr.Message)
	case errors.As(err, &pgErr) && pgErr.Code == "23505":
		erroJSON(w, http.StatusConflict, "duplicado", "já existe")
	case errors.As(err, &pgErr) && pgErr.Code == "22P02":
		erroJSON(w, http.StatusNotFound, "nao_encontrado", "não encontrado")
	default:
		slog.Error("erro", "rota", r.URL.Path, "erro", err)
		erroJSON(w, http.StatusInternalServerError, "interno", "erro interno")
	}
}

var (
	errValidacao     = errors.New("dados inválidos")
	errNaoEncontrado = errors.New("não encontrado")
)

type erroCampo struct{ msg string }

func (e erroCampo) Error() string { return e.msg }
func (e erroCampo) Unwrap() error { return errValidacao }

func invalido(msg string) error { return erroCampo{msg} }

func mensagemValidacao(err error) string {
	var c erroCampo
	if errors.As(err, &c) {
		return c.msg
	}
	return err.Error()
}
