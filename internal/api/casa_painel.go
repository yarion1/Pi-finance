package api

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/financeiro"
)

// GET /api/casas/{id}/painel?mes=AAAA-MM: consolidado, quem deve quem e metas conjuntas.
func (s *Servidor) painelCasa(w http.ResponseWriter, r *http.Request) {
	mes, err := mesQuery(r)
	if err != nil {
		falhar(w, r, err)
		return
	}
	var p financeiro.PainelCasa
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		p, err = financeiro.MontarPainelCasa(ctx, tx, r.PathValue("id"), mes, financeiro.Hoje())
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, p)
}

// POST /api/casas/{id}/divisoes: divide um gasto da pessoa com gente da casa.
func (s *Servidor) dividirDespesa(w http.ResponseWriter, r *http.Request) {
	var p financeiro.PedidoDivisao
	if !lerJSON(w, r, &p) {
		return
	}
	var id string
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		id, err = financeiro.DividirDespesa(ctx, tx, r.PathValue("id"), p)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// DELETE /api/divisoes/{id}: só quem pagou desfaz a divisão.
func (s *Servidor) apagarDivisao(w http.ResponseWriter, r *http.Request) {
	if err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return financeiro.ApagarDespesaDividida(ctx, tx, r.PathValue("id"))
	}); err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// POST /api/casas/{id}/acertos {usuario_id}: acerta tudo com a pessoa.
func (s *Servidor) acertar(w http.ResponseWriter, r *http.Request) {
	var c struct {
		UsuarioID string `json:"usuario_id"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	var a financeiro.Acerto
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		if a, err = financeiro.Acertar(ctx, tx, r.PathValue("id"), c.UsuarioID); err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "acerto", r.PathValue("id"), map[string]any{"com": c.UsuarioID, "valor_centavos": a.Valor})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, a)
}

// POST /api/casas/{id}/metas: meta conjunta.
func (s *Servidor) criarMetaCasa(w http.ResponseWriter, r *http.Request) {
	var d financeiro.DadosMetaCasa
	if !lerJSON(w, r, &d) {
		return
	}
	var id string
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		id, err = financeiro.CriarMetaCasa(ctx, tx, r.PathValue("id"), d)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// POST /api/metas/{id}/contribuicoes {valor_centavos, data}
func (s *Servidor) contribuirMeta(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Valor int64  `json:"valor_centavos"`
		Data  string `json:"data"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	data := financeiro.Hoje()
	if c.Data != "" {
		d, err := time.Parse("2006-01-02", c.Data)
		if err != nil {
			falhar(w, r, invalido("data no formato AAAA-MM-DD"))
			return
		}
		data = d
	}
	var id string
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		id, err = financeiro.Contribuir(ctx, tx, r.PathValue("id"), c.Valor, data)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

// DELETE /api/contribuicoes/{id}: só a própria.
func (s *Servidor) apagarContribuicao(w http.ResponseWriter, r *http.Request) {
	s.apagarDaTabela(w, r, "contribuicoes_meta")
}
