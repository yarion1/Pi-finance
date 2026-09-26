package api

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/financeiro"
)

// GET /api/relatorios: relatórios do mês e resumos da semana (sem os dados).
func (s *Servidor) listarRelatorios(w http.ResponseWriter, r *http.Request) {
	var lista []financeiro.RelatorioSalvo
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		lista, err = financeiro.ListarRelatorios(ctx, tx)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

// GET /api/relatorios/{id}
func (s *Servidor) lerRelatorio(w http.ResponseWriter, r *http.Request) {
	var rel financeiro.RelatorioSalvo
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		rel, err = financeiro.LerRelatorio(ctx, tx, r.PathValue("id"))
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, rel)
}

// POST /api/relatorios/gerar: o que o worker faria agora (mês fechado e última semana).
func (s *Servidor) gerarRelatorios(w http.ResponseWriter, r *http.Request) {
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2 * time.Minute))
	ctx, cancelar := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancelar()
	novos, err := financeiro.GerarRelatorios(ctx, s.IA, financeiro.Agora(), func(fn func(ctx context.Context, tx pgx.Tx) error) error {
		return s.comUsuario(r, fn)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	if novos == nil {
		novos = []string{}
	}
	escreverJSON(w, http.StatusOK, map[string]any{"novos": novos})
}
