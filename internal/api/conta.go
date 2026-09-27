package api

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/auth"
	"github.com/yarion1/pi-finance/internal/financeiro"
)

// PrazoExclusao: a conta some de verdade depois disso (dá para cancelar até lá).
const PrazoExclusao = 30 * 24 * time.Hour

// POST /api/conta/exportar {formato: json|planilha}: todos os dados da pessoa. Pede a
// senha de novo e fica na auditoria.
func (s *Servidor) exportarDados(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Formato string `json:"formato"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	if c.Formato != "json" && c.Formato != "planilha" {
		falhar(w, r, invalido("formato: json ou planilha"))
		return
	}
	if !sessaoDe(r).Reautenticada(time.Now()) {
		falhar(w, r, auth.ErrReautenticar)
		return
	}
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(2 * time.Minute))
	var corpo []byte
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if c.Formato == "json" {
			e, err := financeiro.ExportarDados(ctx, tx, s.Cifrador, time.Now())
			if err != nil {
				return err
			}
			if corpo, err = json.MarshalIndent(e, "", "  "); err != nil {
				return err
			}
		} else {
			var err error
			if corpo, err = financeiro.PlanilhaDados(ctx, tx); err != nil {
				return err
			}
		}
		return s.auditar(ctx, tx, r, "exportacao", "", map[string]any{"formato": c.Formato, "bytes": len(corpo)})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	nome := "financas-" + time.Now().Format("2006-01-02")
	if c.Formato == "json" {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		nome += ".json"
	} else {
		w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
		nome += ".xlsx"
	}
	w.Header().Set("Content-Disposition", `attachment; filename="`+nome+`"`)
	_, _ = w.Write(corpo)
}

// POST /api/conta/apagar {confirmacao: "APAGAR"}: marca a conta para sumir em 30 dias e
// encerra todas as sessões. Entrar de novo dentro do prazo permite cancelar.
func (s *Servidor) apagarMinhaConta(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Confirmacao string `json:"confirmacao"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	if c.Confirmacao != "APAGAR" {
		falhar(w, r, invalido(`digite APAGAR para confirmar`))
		return
	}
	if !sessaoDe(r).Reautenticada(time.Now()) {
		falhar(w, r, auth.ErrReautenticar)
		return
	}
	quando := time.Now().Add(PrazoExclusao)
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "update usuarios set apagar_em = $1, atualizado_em = now() where id = app_usuario_id()", quando); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, "delete from sessoes where usuario_id = app_usuario_id()"); err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "exclusao_pedida", "", map[string]any{"apagar_em": quando.Format(time.RFC3339)})
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	s.apagarCookie(w, cookieSessao)
	escreverJSON(w, http.StatusOK, map[string]any{"apagar_em": quando})
}

// POST /api/conta/cancelar-exclusao
func (s *Servidor) cancelarExclusao(w http.ResponseWriter, r *http.Request) {
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "update usuarios set apagar_em = null, atualizado_em = now() where id = app_usuario_id()"); err != nil {
			return err
		}
		return s.auditar(ctx, tx, r, "exclusao_cancelada", "", nil)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// GET /api/conta/atividade: o histórico da própria conta (logins, exportações,
// permissões, exclusões...), os 100 mais recentes.
func (s *Servidor) atividade(w http.ResponseWriter, r *http.Request) {
	type evento struct {
		Acao     string    `json:"acao"`
		IP       *string   `json:"ip"`
		Aparelho *string   `json:"aparelho"`
		Quando   time.Time `json:"quando"`
	}
	var lista []evento
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		linhas, err := tx.Query(ctx, `select acao, ip, dados->>'user_agent', quando from auditoria
			where usuario_id = app_usuario_id() order by quando desc limit 100`)
		if err != nil {
			return err
		}
		lista, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[evento])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	if lista == nil {
		lista = []evento{}
	}
	escreverJSON(w, http.StatusOK, lista)
}

// GET /api/ia/envios: "o que a IA viu", os 100 envios mais recentes.
func (s *Servidor) enviosIA(w http.ResponseWriter, r *http.Request) {
	type envio struct {
		Tipo       string    `json:"tipo"`
		Resumo     string    `json:"resumo"`
		Quando     time.Time `json:"quando"`
		CustoMicro int64     `json:"custo_microdolares"`
	}
	var lista []envio
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		linhas, err := tx.Query(ctx, `select tipo, resumo, quando, custo_microdolares from ia_envios
			order by quando desc limit 100`)
		if err != nil {
			return err
		}
		lista, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[envio])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	if lista == nil {
		lista = []envio{}
	}
	escreverJSON(w, http.StatusOK, lista)
}
