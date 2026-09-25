package api

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/financeiro"
)

type categoria struct {
	ID         string  `json:"id"`
	EntidadeID *string `json:"entidade_id"`
	PaiID      *string `json:"pai_id"`
	Nome       string  `json:"nome"`
	Icone      *string `json:"icone"`
	Cor        *string `json:"cor"`
	Tipo       string  `json:"tipo"`
	Ordem      int16   `json:"ordem"`
}

// listarCategorias: as padrão e as das entidades visíveis (ou só as de uma).
func (s *Servidor) listarCategorias(w http.ResponseWriter, r *http.Request) {
	var lista []categoria
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		consulta := "select id, entidade_id, pai_id, nome, icone, cor, tipo::text, ordem from categorias"
		args := []any{}
		if e := r.URL.Query().Get("entidade_id"); e != "" {
			consulta += " where entidade_id is null or entidade_id = $1"
			args = append(args, e)
		}
		linhas, err := tx.Query(ctx, consulta+" order by tipo, pai_id nulls first, ordem, nome", args...)
		if err != nil {
			return err
		}
		lista, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[categoria])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

var reCor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

func (s *Servidor) criarCategoria(w http.ResponseWriter, r *http.Request) {
	var c struct {
		EntidadeID string  `json:"entidade_id"`
		PaiID      *string `json:"pai_id"`
		Nome       string  `json:"nome"`
		Tipo       string  `json:"tipo"`
		Cor        *string `json:"cor"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	nome, err := validarNome(c.Nome)
	if err != nil {
		falhar(w, r, err)
		return
	}
	if c.PaiID != nil && *c.PaiID == "" {
		c.PaiID = nil
	}
	if c.PaiID == nil && c.Tipo != "gasto" && c.Tipo != "receita" && c.Tipo != "transferencia" {
		falhar(w, r, invalido("tipo: gasto, receita ou transferencia"))
		return
	}
	if c.Tipo == "" {
		c.Tipo = "gasto" // subcategoria herda o tipo da mãe (gatilho)
	}
	if c.Cor != nil && !reCor.MatchString(*c.Cor) {
		falhar(w, r, invalido("cor no formato #rrggbb"))
		return
	}
	var id string
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `insert into categorias (entidade_id, pai_id, nome, tipo, cor) values ($1, $2, $3, $4, $5)
			returning id`, c.EntidadeID, c.PaiID, nome, c.Tipo, c.Cor).Scan(&id)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Servidor) editarCategoria(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Nome string  `json:"nome"`
		Cor  *string `json:"cor"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	nome, err := validarNome(c.Nome)
	if err != nil {
		falhar(w, r, err)
		return
	}
	if c.Cor != nil && !reCor.MatchString(*c.Cor) {
		falhar(w, r, invalido("cor no formato #rrggbb"))
		return
	}
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return exigirUma(tx.Exec(ctx, "update categorias set nome = $2, cor = coalesce($3, cor) where id = $1",
			r.PathValue("id"), nome, c.Cor))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Servidor) apagarCategoria(w http.ResponseWriter, r *http.Request) {
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return exigirUma(tx.Exec(ctx, "delete from categorias where id = $1", r.PathValue("id")))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type regraCategoria struct {
	ID          string    `json:"id"`
	EntidadeID  string    `json:"entidade_id"`
	Texto       string    `json:"texto"`
	ContaID     *string   `json:"conta_id"`
	CategoriaID string    `json:"categoria_id"`
	Categoria   string    `json:"categoria"`
	Prioridade  int       `json:"prioridade"`
	CriadaEm    time.Time `json:"criada_em"`
}

func (s *Servidor) listarRegras(w http.ResponseWriter, r *http.Request) {
	var lista []regraCategoria
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		consulta := `select r.id, r.entidade_id, r.texto, r.conta_id, r.categoria_id,
			coalesce(p.nome || ' › ', '') || c.nome, r.prioridade, r.criada_em
			from regras_categoria r join categorias c on c.id = r.categoria_id left join categorias p on p.id = c.pai_id`
		args := []any{}
		if e := r.URL.Query().Get("entidade_id"); e != "" {
			consulta += " where r.entidade_id = $1"
			args = append(args, e)
		}
		linhas, err := tx.Query(ctx, consulta+" order by r.prioridade desc, r.texto", args...)
		if err != nil {
			return err
		}
		lista, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[regraCategoria])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

func (s *Servidor) criarRegra(w http.ResponseWriter, r *http.Request) {
	var c struct {
		EntidadeID  string  `json:"entidade_id"`
		Texto       string  `json:"texto"`
		ContaID     *string `json:"conta_id"`
		CategoriaID string  `json:"categoria_id"`
		Prioridade  int     `json:"prioridade"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	texto := strings.TrimSpace(c.Texto)
	if texto == "" || len([]rune(texto)) > 100 {
		falhar(w, r, invalido("o texto da regra tem de 1 a 100 caracteres"))
		return
	}
	if c.ContaID != nil && *c.ContaID == "" {
		c.ContaID = nil
	}
	var id string
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `insert into regras_categoria (entidade_id, texto, conta_id, categoria_id, prioridade)
			values ($1, $2, $3, $4, $5) returning id`, c.EntidadeID, texto, c.ContaID, c.CategoriaID, c.Prioridade).Scan(&id)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Servidor) apagarRegra(w http.ResponseWriter, r *http.Request) {
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return exigirUma(tx.Exec(ctx, "delete from regras_categoria where id = $1", r.PathValue("id")))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// aplicarRegras categoriza as transações sem categoria da entidade.
func (s *Servidor) aplicarRegras(w http.ResponseWriter, r *http.Request) {
	var c struct {
		EntidadeID string `json:"entidade_id"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	n := 0
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		var pode bool
		if err := tx.QueryRow(ctx, "select app_pode_editar_entidade($1)", c.EntidadeID).Scan(&pode); err != nil {
			return err
		}
		if !pode {
			return errNaoEncontrado
		}
		cat, err := financeiro.NovoCategorizador(ctx, tx, c.EntidadeID)
		if err != nil {
			return err
		}
		linhas, err := tx.Query(ctx, `select id, conta_id, descricao, valor_centavos from transacoes
			where entidade_id = $1 and categoria_id is null and tipo in ('gasto', 'receita')`, c.EntidadeID)
		if err != nil {
			return err
		}
		type pendente struct {
			id, conta, desc string
			valor           int64
		}
		var lista []pendente
		for linhas.Next() {
			var p pendente
			if err := linhas.Scan(&p.id, &p.conta, &p.desc, &p.valor); err != nil {
				linhas.Close()
				return err
			}
			lista = append(lista, p)
		}
		linhas.Close()
		for _, p := range lista {
			if categoria, _ := cat.Sugerir(p.conta, p.desc, core.Centavos(p.valor)); categoria != nil {
				if err := recategorizar(ctx, tx, p.id, categoria); err != nil {
					return err
				}
				n++
			}
		}
		return nil
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, map[string]int{"categorizadas": n})
}

type mapeamento struct {
	ID       string          `json:"id"`
	Nome     string          `json:"nome"`
	Config   json.RawMessage `json:"config"`
	CriadoEm time.Time       `json:"criado_em"`
}

func (s *Servidor) listarMapeamentos(w http.ResponseWriter, r *http.Request) {
	var lista []mapeamento
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		linhas, err := tx.Query(ctx, "select id, nome, config, criado_em from mapeamentos_csv order by nome")
		if err != nil {
			return err
		}
		lista, err = pgx.CollectRows(linhas, pgx.RowToStructByPos[mapeamento])
		return err
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusOK, lista)
}

func (s *Servidor) salvarMapeamento(w http.ResponseWriter, r *http.Request) {
	var c struct {
		Nome   string          `json:"nome"`
		Config json.RawMessage `json:"config"`
	}
	if !lerJSON(w, r, &c) {
		return
	}
	nome, err := validarNome(c.Nome)
	if err != nil || len(c.Config) == 0 {
		falhar(w, r, invalido("nome e configuração do mapeamento"))
		return
	}
	var id string
	err = s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `insert into mapeamentos_csv (usuario_id, nome, config) values (app_usuario_id(), $1, $2)
			on conflict (usuario_id, nome) do update set config = excluded.config returning id`, nome, c.Config).Scan(&id)
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	escreverJSON(w, http.StatusCreated, map[string]string{"id": id})
}

func (s *Servidor) apagarMapeamento(w http.ResponseWriter, r *http.Request) {
	err := s.comUsuario(r, func(ctx context.Context, tx pgx.Tx) error {
		return exigirUma(tx.Exec(ctx, "delete from mapeamentos_csv where id = $1", r.PathValue("id")))
	})
	if err != nil {
		falhar(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
