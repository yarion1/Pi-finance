package financeiro

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

// Consultas usadas pelas telas e pelas ferramentas da IA: a mesma função responde aos
// dois, para o chat nunca dar um número diferente da tela (critério da fase 6).

// GastoCategoria: total gasto numa categoria-mãe no período.
type GastoCategoria struct {
	CategoriaID *string `json:"categoria_id"`
	Nome        string  `json:"nome"`
	Cor         *string `json:"cor"`
	Total       int64   `json:"total_centavos"`
}

// GastosPorCategoria em reais entre de e ate (inclusive), agrupados pela categoria-mãe;
// estornos abatem e transferências ficam de fora.
func GastosPorCategoria(ctx context.Context, tx pgx.Tx, ents []string, de, ate time.Time) ([]GastoCategoria, error) {
	linhas, err := tx.Query(ctx, `select coalesce(pai.id, cat.id), coalesce(pai.nome, cat.nome, 'Sem categoria'),
			coalesce(pai.cor, cat.cor), -sum(t.valor_centavos) as total
		from transacoes t
		left join categorias cat on cat.id = t.categoria_id
		left join categorias pai on pai.id = cat.pai_id
		where t.tipo in ('gasto', 'estorno') and t.moeda = 'BRL' and t.entidade_id = any($1)
		  and t.data between $2 and $3
		group by 1, 2, 3 having -sum(t.valor_centavos) > 0 order by total desc`, ents, de, ate)
	if err != nil {
		return nil, err
	}
	lista, err := pgx.CollectRows(linhas, pgx.RowToStructByPos[GastoCategoria])
	if lista == nil {
		lista = []GastoCategoria{}
	}
	return lista, err
}

// TotaisPeriodo: receitas e gastos (positivos) em reais no período.
func TotaisPeriodo(ctx context.Context, tx pgx.Tx, ents []string, de, ate time.Time) (receitas, gastos int64, err error) {
	err = tx.QueryRow(ctx, `select
			coalesce(sum(valor_centavos) filter (where tipo = 'receita'), 0),
			coalesce(-sum(valor_centavos) filter (where tipo in ('gasto', 'estorno')), 0)
		from transacoes t where t.moeda = 'BRL' and t.entidade_id = any($1) and t.data between $2 and $3`,
		ents, de, ate).Scan(&receitas, &gastos)
	return receitas, gastos, err
}

// TransacaoEncontrada na busca.
type TransacaoEncontrada struct {
	ID        string  `json:"id"`
	Data      string  `json:"data"`
	Descricao string  `json:"descricao"`
	Valor     int64   `json:"valor_centavos"`
	Tipo      string  `json:"tipo"`
	Conta     string  `json:"conta"`
	Categoria *string `json:"categoria"`
}

// BuscarTransacoes pelo texto da descrição (sem diferenciar maiúsculas) no período, as mais
// recentes primeiro; categoria filtra pelo nome da categoria ou da mãe.
func BuscarTransacoes(ctx context.Context, tx pgx.Tx, ents []string, texto, categoria string, de, ate time.Time, limite int) ([]TransacaoEncontrada, error) {
	if limite <= 0 || limite > 200 {
		limite = 50
	}
	linhas, err := tx.Query(ctx, `select t.id, to_char(t.data, 'YYYY-MM-DD'), t.descricao, t.valor_centavos, t.tipo::text,
			c.nome, coalesce(pai.nome || ' › ' || cat.nome, cat.nome)
		from transacoes t join contas c on c.id = t.conta_id
		left join categorias cat on cat.id = t.categoria_id
		left join categorias pai on pai.id = cat.pai_id
		where t.entidade_id = any($1) and t.data between $2 and $3
		  and ($4 = '' or t.descricao ilike '%' || $4 || '%' or t.descricao_original ilike '%' || $4 || '%')
		  and ($5 = '' or cat.nome ilike $5 or pai.nome ilike $5)
		order by t.data desc, t.criada_em desc limit $6`,
		ents, de, ate, strings.TrimSpace(texto), strings.TrimSpace(categoria), limite)
	if err != nil {
		return nil, err
	}
	lista, err := pgx.CollectRows(linhas, pgx.RowToStructByPos[TransacaoEncontrada])
	if lista == nil {
		lista = []TransacaoEncontrada{}
	}
	return lista, err
}
