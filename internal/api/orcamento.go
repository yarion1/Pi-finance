package api

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/financeiro"
)

func (s *Servidor) orcamento(w http.ResponseWriter, r *http.Request) {
	var resp financeiro.Orcamento
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		resp, err = financeiro.CalcularOrcamento(ctx, tx, r.URL.Query().Get("entidade_id"), r.URL.Query().Get("mes"), financeiro.Hoje())
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, resp)
}

func (s *Servidor) salvarOrcamento(w http.ResponseWriter, r *http.Request) {
	var c struct {
		EntidadeID string `json:"entidade_id"`
		Mes        string `json:"mes"`
		Itens      []struct {
			CategoriaID string `json:"categoria_id"`
			Limite      int64  `json:"limite_centavos"`
		} `json:"itens"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	mes, err := time.Parse("2006-01", c.Mes)
	if err != nil {
		falhar(w, r, invalido("mês no formato AAAA-MM"))
		return
	}
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		manter := []string{}
		for _, it := range c.Itens {
			if it.Limite < 0 {
				return invalido("limite não pode ser negativo")
			}
			if _, err := tx.Exec(ctx, `insert into orcamentos (entidade_id, mes, categoria_id, limite_centavos) values ($1, $2, $3, $4)
				on conflict (entidade_id, mes, categoria_id) do update set limite_centavos = excluded.limite_centavos`,
				c.EntidadeID, mes, it.CategoriaID, it.Limite); err != nil {
				return err
			}
			manter = append(manter, it.CategoriaID)
		}
		_, err := tx.Exec(ctx, "delete from orcamentos where entidade_id = $1 and mes = $2 and not (categoria_id = any($3::uuid[]))",
			c.EntidadeID, mes, manter)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) copiarOrcamento(w http.ResponseWriter, r *http.Request) {
	var c struct {
		EntidadeID string `json:"entidade_id"`
		De         string `json:"de"`
		Para       string `json:"para"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	de, err1 := time.Parse("2006-01", c.De)
	para, err2 := time.Parse("2006-01", c.Para)
	if err1 != nil || err2 != nil {
		falhar(w, r, invalido("meses no formato AAAA-MM"))
		return
	}
	var copiados int64
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `insert into orcamentos (entidade_id, mes, categoria_id, limite_centavos)
			select entidade_id, $3, categoria_id, limite_centavos from orcamentos where entidade_id = $1 and mes = $2
			on conflict (entidade_id, mes, categoria_id) do nothing`, c.EntidadeID, de, para)
		copiados = tag.RowsAffected()
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, map[string]int64{"copiados": copiados})
}

func (s *Servidor) configurarOrcamento(w http.ResponseWriter, r *http.Request) {
	var c struct {
		EntidadeID string            `json:"entidade_id"`
		Modo       string            `json:"modo"`
		Grupos     map[string]string `json:"grupos"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	if c.Modo != "categoria" && c.Modo != "envelopes" && c.Modo != "50_30_20" {
		falhar(w, r, invalido("modo: categoria, envelopes ou 50_30_20"))
		return
	}
	for _, g := range c.Grupos {
		if g != "necessidades" && g != "desejos" {
			falhar(w, r, invalido("grupo: necessidades ou desejos"))
			return
		}
	}
	if c.Grupos == nil {
		c.Grupos = map[string]string{}
	}
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `insert into orcamento_config (entidade_id, modo, grupos) values ($1, $2, $3)
			on conflict (entidade_id) do update set modo = excluded.modo, grupos = excluded.grupos`, c.EntidadeID, c.Modo, c.Grupos)
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type compromisso struct {
	ID          string  `json:"id"`
	EntidadeID  string  `json:"entidade_id"`
	ContaID     *string `json:"conta_id"`
	Descricao   string  `json:"descricao"`
	Valor       int64   `json:"valor_centavos"`
	Vencimento  string  `json:"vencimento"`
	CategoriaID *string `json:"categoria_id"`
	PagoEm      *string `json:"pago_em"`
}

func (s *Servidor) listarCompromissos(w http.ResponseWriter, r *http.Request) {
	var lista []compromisso
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		ents, err := financeiro.Entidades(ctx, tx, r.URL.Query().Get("entidade_id"))
		if err != nil {
			return err
		}
		linhas, err := tx.Query(ctx, `select id, entidade_id, conta_id, descricao, valor_centavos, to_char(vencimento, 'YYYY-MM-DD'),
				categoria_id, to_char(pago_em, 'YYYY-MM-DD')
			from compromissos where entidade_id = any($1) and (pago_em is null or pago_em >= current_date - 60)
			order by pago_em nulls first, vencimento`, ents)
		if err != nil {
			return err
		}
		lista, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[compromisso])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	if lista == nil {
		lista = []compromisso{}
	}
	escreverJSON(w, http.StatusOK, lista)
}
