package api

import (
	"context"
	"net/http"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/financeiro"
)

// carteira: os ativos com posição e valor de hoje, e o total por classe.
func (s *Servidor) carteira(w http.ResponseWriter, r *http.Request) {
	var c financeiro.Carteira
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		ents, err := financeiro.Entidades(ctx, tx, r.URL.Query().Get("entidade_id"))
		if err != nil {
			return err
		}
		c, err = financeiro.CalcularCarteira(ctx, tx, ents, financeiro.Hoje())
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, c)
}
