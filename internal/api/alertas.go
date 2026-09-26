package api

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
)

type alerta struct {
	ID       string         `json:"id"`
	Tipo     string         `json:"tipo"`
	Dados    map[string]any `json:"dados"`
	CriadoEm time.Time      `json:"criado_em"`
	LidoEm   *time.Time     `json:"lido_em"`
}

func (s *Servidor) listarAlertas(w http.ResponseWriter, r *http.Request) {
	var lista []alerta
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		// só os não dispensados; ?todos=1 traz também os lidos
		todos := r.URL.Query().Get("todos") == "1"
		linhas, err := tx.Query(ctx, `select id, tipo, dados, criado_em, lido_em from alertas
			where $1 or lido_em is null order by criado_em desc limit 50`, todos)
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

// dispensarAlerta marca um alerta como lido (some da lista; não volta a ser criado).
func (s *Servidor) dispensarAlerta(w http.ResponseWriter, r *http.Request) {
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return exigirUma(tx.Exec(ctx, "update alertas set lido_em = coalesce(lido_em, now()) where id = $1", r.PathValue("id")))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// dispensarAlertas marca todos os alertas da pessoa como lidos.
func (s *Servidor) dispensarAlertas(w http.ResponseWriter, r *http.Request) {
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, "update alertas set lido_em = now() where lido_em is null")
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
