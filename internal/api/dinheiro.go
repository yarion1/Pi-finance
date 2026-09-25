package api

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

// Leitura mínima de contas e transações. Cadastro, importação e edição chegam
// na fase 1; aqui elas existem para o isolamento ser testado por rota.

type conta struct {
	ID           string  `json:"id"`
	EntidadeID   string  `json:"entidade_id"`
	Nome         string  `json:"nome"`
	Tipo         string  `json:"tipo"`
	Moeda        string  `json:"moeda"`
	Visibilidade string  `json:"visibilidade"`
	CasaID       *string `json:"casa_id"`
}

type transacao struct {
	ID            string `json:"id"`
	ContaID       string `json:"conta_id"`
	Data          string `json:"data"`
	Descricao     string `json:"descricao"`
	ValorCentavos int64  `json:"valor_centavos"`
	Moeda         string `json:"moeda"`
	Tipo          string `json:"tipo"`
}

type alerta struct {
	ID       string         `json:"id"`
	Tipo     string         `json:"tipo"`
	Dados    map[string]any `json:"dados"`
	CriadoEm time.Time      `json:"criado_em"`
	LidoEm   *time.Time     `json:"lido_em"`
}

func (s *Servidor) listarContas(w http.ResponseWriter, r *http.Request) {
	var lista []conta
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		linhas, err := tx.Query(ctx, `select id, entidade_id, nome, tipo::text, moeda, visibilidade::text, casa_id
			from contas order by nome`)
		if err != nil {
			return err
		}
		lista, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[conta])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

func (s *Servidor) listarTransacoes(w http.ResponseWriter, r *http.Request) {
	var lista []transacao
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		consulta := `select id, conta_id, to_char(data, 'YYYY-MM-DD'), descricao, valor_centavos, moeda, tipo::text
			from transacoes`
		args := []any{}
		if c := r.URL.Query().Get("conta_id"); c != "" {
			consulta += " where conta_id = $1"
			args = append(args, c)
		}
		linhas, err := tx.Query(ctx, consulta+" order by data desc, criada_em desc limit 200", args...)
		if err != nil {
			return err
		}
		lista, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[transacao])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

func (s *Servidor) listarAlertas(w http.ResponseWriter, r *http.Request) {
	var lista []alerta
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		linhas, err := tx.Query(ctx, "select id, tipo, dados, criado_em, lido_em from alertas order by criado_em desc limit 50")
		if err != nil {
			return err
		}
		lista, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[alerta])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}
