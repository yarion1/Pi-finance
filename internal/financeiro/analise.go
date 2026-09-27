package financeiro

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
)

// SerieCategoria: gasto da categoria mãe em cada mês da janela.
type SerieCategoria struct {
	CategoriaID *string `json:"categoria_id"`
	Nome        string  `json:"nome"`
	Cor         *string `json:"cor"`
	Valores     []int64 `json:"valores_centavos"`
}

// DiaGasto do calendário de calor.
type DiaGasto struct {
	Data  string `json:"data"`
	Total int64  `json:"total_centavos"`
}

// AnaliseGastos: a tela Gastos em gráficos.
type AnaliseGastos struct {
	Mes              string                 `json:"mes"`
	Meses            []string               `json:"meses"`
	Series           []SerieCategoria       `json:"series"`
	Treemap          []core.GastoHierarquia `json:"treemap"`
	Dias             []DiaGasto             `json:"dias"`
	Estabelecimentos []core.Estabelecimento `json:"estabelecimentos"`
}

// MaxSeries: categorias com série própria; o resto vira "Outras".
const MaxSeries = 7

// gastosHierarquia: gasto por categoria mãe e filhas no período (sem categoria = "Sem categoria").
func gastosHierarquia(ctx context.Context, tx pgx.Tx, ents []string, de, ate time.Time) ([]core.GastoHierarquia, error) {
	linhas, err := tx.Query(ctx, `select coalesce(pai.id, cat.id)::text, coalesce(pai.nome, cat.nome, 'Sem categoria'),
			coalesce(pai.cor, cat.cor), case when pai.id is null then null else cat.id::text end, cat.nome,
			-sum(t.valor_centavos)
		from transacoes t
		left join categorias cat on cat.id = t.categoria_id
		left join categorias pai on pai.id = cat.pai_id
		where t.tipo in ('gasto', 'estorno') and t.moeda = 'BRL' and t.entidade_id = any($1) and t.data between $2 and $3
		group by 1, 2, 3, 4, 5`, ents, de, ate)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	porMae := map[string]*core.GastoHierarquia{}
	var ordem []string
	for linhas.Next() {
		var maeID, filhaID *string
		var nome string
		var cor, filhaNome *string
		var v int64
		if err := linhas.Scan(&maeID, &nome, &cor, &filhaID, &filhaNome, &v); err != nil {
			return nil, err
		}
		chave := ""
		if maeID != nil {
			chave = *maeID
		}
		g := porMae[chave]
		if g == nil {
			g = &core.GastoHierarquia{ItemValor: core.ItemValor{ID: chave, Nome: nome}, Cor: cor, Filhas: []core.ItemValor{}}
			porMae[chave] = g
			ordem = append(ordem, chave)
		}
		g.Valor += core.Centavos(v)
		if filhaID != nil && filhaNome != nil {
			g.Filhas = append(g.Filhas, core.ItemValor{ID: *filhaID, Nome: *filhaNome, Valor: core.Centavos(v)})
		}
	}
	if err := linhas.Err(); err != nil {
		return nil, err
	}
	out := []core.GastoHierarquia{}
	for _, k := range ordem {
		if porMae[k].Valor > 0 {
			out = append(out, *porMae[k])
		}
	}
	ordenarPorValor(out)
	return out, nil
}

func ordenarPorValor(gs []core.GastoHierarquia) {
	porValor := func(a, b core.ItemValor) int { return int(b.Valor - a.Valor) }
	slices.SortFunc(gs, func(a, b core.GastoHierarquia) int { return porValor(a.ItemValor, b.ItemValor) })
	for _, g := range gs {
		slices.SortFunc(g.Filhas, porValor)
	}
}

// AnalisarGastos: treemap e maiores estabelecimentos do mês; barras por categoria nos
// `meses` anteriores (até o mês); calendário dos últimos 365 dias até o fim do mês.
func AnalisarGastos(ctx context.Context, tx pgx.Tx, ents []string, mes time.Time, meses int) (AnaliseGastos, error) {
	a := AnaliseGastos{Mes: mes.Format("2006-01"), Meses: []string{}, Series: []SerieCategoria{}, Dias: []DiaGasto{}}
	fim := mes.AddDate(0, 1, -1)
	var err error
	if a.Treemap, err = gastosHierarquia(ctx, tx, ents, mes, fim); err != nil {
		return a, err
	}

	// estabelecimentos do mês
	linhas, err := tx.Query(ctx, `select descricao, valor_centavos from transacoes where entidade_id = any($1)
		and tipo = 'gasto' and moeda = 'BRL' and data between $2 and $3`, ents, mes, fim)
	if err != nil {
		return a, err
	}
	var descs []string
	var vals []core.Centavos
	for linhas.Next() {
		var d string
		var v int64
		if err := linhas.Scan(&d, &v); err != nil {
			linhas.Close()
			return a, err
		}
		descs, vals = append(descs, d), append(vals, core.Centavos(v))
	}
	linhas.Close()
	a.Estabelecimentos = core.AgruparEstabelecimentos(descs, vals, 10)

	// barras empilhadas: as maiores categorias da janela têm série própria
	inicio := mes.AddDate(0, -(meses - 1), 0)
	for m := inicio; !m.After(mes); m = m.AddDate(0, 1, 0) {
		a.Meses = append(a.Meses, m.Format("2006-01"))
	}
	janela, err := GastosPorCategoria(ctx, tx, ents, inicio, fim)
	if err != nil {
		return a, err
	}
	idx := map[string]int{}
	outras := -1
	for i, c := range janela {
		if i == MaxSeries {
			outras = len(a.Series)
			a.Series = append(a.Series, SerieCategoria{Nome: "Outras", Valores: make([]int64, meses)})
			break
		}
		a.Series = append(a.Series, SerieCategoria{CategoriaID: c.CategoriaID, Nome: c.Nome, Cor: c.Cor, Valores: make([]int64, meses)})
		idx[c.Nome] = i
	}
	linhas, err = tx.Query(ctx, `select to_char(t.data, 'YYYY-MM'), coalesce(pai.nome, cat.nome, 'Sem categoria'), -sum(t.valor_centavos)
		from transacoes t
		left join categorias cat on cat.id = t.categoria_id
		left join categorias pai on pai.id = cat.pai_id
		where t.tipo in ('gasto', 'estorno') and t.moeda = 'BRL' and t.entidade_id = any($1) and t.data between $2 and $3
		group by 1, 2`, ents, inicio, fim)
	if err != nil {
		return a, err
	}
	posMes := map[string]int{}
	for i, m := range a.Meses {
		posMes[m] = i
	}
	for linhas.Next() {
		var m, nome string
		var v int64
		if err := linhas.Scan(&m, &nome, &v); err != nil {
			linhas.Close()
			return a, err
		}
		i, ok := idx[nome]
		if !ok {
			i = outras
		}
		if i >= 0 {
			a.Series[i].Valores[posMes[m]] += v
		}
	}
	linhas.Close()

	// calendário
	linhas, err = tx.Query(ctx, `select to_char(data, 'YYYY-MM-DD'), -sum(valor_centavos) from transacoes
		where entidade_id = any($1) and tipo in ('gasto', 'estorno') and moeda = 'BRL' and data between $2 and $3
		group by 1 having -sum(valor_centavos) > 0 order by 1`, ents, fim.AddDate(0, 0, -364), fim)
	if err != nil {
		return a, err
	}
	dias, err := pgx.CollectRows(linhas, pgx.RowToStructByPos[DiaGasto])
	if err != nil {
		return a, err
	}
	if dias != nil {
		a.Dias = dias
	}
	return a, nil
}

// Cascata do mês nas contas do dia a dia.
type Cascata struct {
	SaldoInicial int64 `json:"saldo_inicial_centavos"`
	Entradas     int64 `json:"entradas_centavos"`
	Saidas       int64 `json:"saidas_centavos"`
	SaldoFinal   int64 `json:"saldo_final_centavos"`
}

// AnaliseFluxo: o Sankey e a cascata do mês.
type AnaliseFluxo struct {
	Mes      string               `json:"mes"`
	Nos      []core.NoSankey      `json:"nos"`
	Ligacoes []core.LigacaoSankey `json:"ligacoes"`
	Cascata  Cascata              `json:"cascata"`
}

// AnalisarFluxo do mês.
func AnalisarFluxo(ctx context.Context, tx pgx.Tx, ents []string, mes time.Time) (AnaliseFluxo, error) {
	a := AnaliseFluxo{Mes: mes.Format("2006-01")}
	fim := mes.AddDate(0, 1, -1)
	linhas, err := tx.Query(ctx, `select coalesce(pai.id, cat.id)::text, coalesce(pai.nome, cat.nome, 'Outras entradas'),
			sum(t.valor_centavos)
		from transacoes t
		left join categorias cat on cat.id = t.categoria_id
		left join categorias pai on pai.id = cat.pai_id
		where t.tipo = 'receita' and t.moeda = 'BRL' and t.entidade_id = any($1) and t.data between $2 and $3
		group by 1, 2 order by 3 desc`, ents, mes, fim)
	if err != nil {
		return a, err
	}
	var receitas []core.ItemValor
	for linhas.Next() {
		var id *string
		var r core.ItemValor
		var v int64
		if err := linhas.Scan(&id, &r.Nome, &v); err != nil {
			linhas.Close()
			return a, err
		}
		if id != nil {
			r.ID = *id
		}
		r.Valor = core.Centavos(v)
		receitas = append(receitas, r)
	}
	linhas.Close()
	gastos, err := gastosHierarquia(ctx, tx, ents, mes, fim)
	if err != nil {
		return a, err
	}
	a.Nos, a.Ligacoes = core.MontarSankey(receitas, gastos)

	// cascata: saldo das contas do dia a dia no fim do mês anterior e no fim deste
	saldo := func(data time.Time) (int64, error) {
		var s int64
		err := tx.QueryRow(ctx, `select coalesce(sum(app_saldo_conta(id, $2)), 0) from contas
			where entidade_id = any($1) and moeda = 'BRL' and tipo in ('corrente', 'poupanca', 'carteira', 'dinheiro', 'beneficio')`,
			ents, data).Scan(&s)
		return s, err
	}
	if a.Cascata.SaldoInicial, err = saldo(mes.AddDate(0, 0, -1)); err != nil {
		return a, err
	}
	if a.Cascata.SaldoFinal, err = saldo(fim); err != nil {
		return a, err
	}
	err = tx.QueryRow(ctx, `select coalesce(sum(t.valor_centavos) filter (where t.valor_centavos > 0), 0),
			coalesce(-sum(t.valor_centavos) filter (where t.valor_centavos < 0), 0)
		from transacoes t join contas c on c.id = t.conta_id
		where t.entidade_id = any($1) and t.moeda = 'BRL' and t.data between $2 and $3
		  and c.tipo in ('corrente', 'poupanca', 'carteira', 'dinheiro', 'beneficio')`, ents, mes, fim).
		Scan(&a.Cascata.Entradas, &a.Cascata.Saidas)
	return a, err
}
