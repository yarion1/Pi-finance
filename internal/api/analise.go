package api

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/financeiro"
)

// mesQuery: ?mes=AAAA-MM (padrão: o mês de hoje), como primeiro dia do mês.
func mesQuery(r *http.Request) (time.Time, error) {
	v := r.URL.Query().Get("mes")
	if v == "" {
		h := financeiro.Hoje()
		return time.Date(h.Year(), h.Month(), 1, 0, 0, 0, 0, time.UTC), nil
	}
	m, err := time.Parse("2006-01", v)
	if err != nil {
		return m, invalido("mês no formato AAAA-MM")
	}
	return m, nil
}

// GET /api/analise/gastos?mes=&meses=12: treemap, barras por mês, calendário e estabelecimentos.
func (s *Servidor) analiseGastos(w http.ResponseWriter, r *http.Request) {
	var a financeiro.AnaliseGastos
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		mes, err := mesQuery(r)
		if err != nil {
			return err
		}
		meses, err := inteiroQuery(r, "meses", 12)
		if err != nil || meses < 1 || meses > 60 {
			return invalido("meses de 1 a 60")
		}
		ents, err := financeiro.Entidades(ctx, tx, r.URL.Query().Get("entidade_id"))
		if err != nil {
			return err
		}
		a, err = financeiro.AnalisarGastos(ctx, tx, ents, mes, int(meses))
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, a)
}

// GET /api/analise/fluxo?mes=: Sankey das entradas às categorias e a cascata do saldo.
func (s *Servidor) analiseFluxo(w http.ResponseWriter, r *http.Request) {
	var a financeiro.AnaliseFluxo
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		mes, err := mesQuery(r)
		if err != nil {
			return err
		}
		ents, err := financeiro.Entidades(ctx, tx, r.URL.Query().Get("entidade_id"))
		if err != nil {
			return err
		}
		a, err = financeiro.AnalisarFluxo(ctx, tx, ents, mes)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, a)
}
