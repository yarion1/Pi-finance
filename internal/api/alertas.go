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
