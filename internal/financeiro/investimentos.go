package financeiro

import (
	"context"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
)

// AtivoCarteira é um ativo com a posição e o valor de hoje.
type AtivoCarteira struct {
	ID          string        `json:"id"`
	EntidadeID  string        `json:"entidade_id"`
	Codigo      string        `json:"codigo"`
	Nome        string        `json:"nome"`
	Classe      string        `json:"classe"`
	Subtipo     *string       `json:"subtipo"`
	Instituicao *string       `json:"instituicao"`
	Emissor     *string       `json:"emissor"`
	Vencimento  *string       `json:"vencimento"`
	Origem      string        `json:"origem"`
	Quantidade  string        `json:"quantidade"`
	PrecoMedio  string        `json:"preco_medio"`
	Cotacao     *string       `json:"cotacao"`
	CotacaoEm   *string       `json:"cotacao_em"`
	Custo       core.Centavos `json:"custo_centavos"`
	Valor       core.Centavos `json:"valor_centavos"`
	Rendimento  core.Centavos `json:"rendimento_centavos"` // valor − custo (sem proventos)
	Proventos   core.Centavos `json:"proventos_centavos"`
	Percentual  float64       `json:"percentual"` // do total da carteira
	Encerrado   bool          `json:"encerrado"`
	Isento      bool          `json:"isento_ir"`
	// XIRR ao ano desde a primeira operação (só com 30 dias ou mais de história)
	Rentabilidade *float64 `json:"rentabilidade_aa"`
	NaCurva       bool     `json:"na_curva"` // renda fixa sem cotação: valor pela taxa contratada
}

// ClasseCarteira: total por classe.
type ClasseCarteira struct {
	Classe     string        `json:"classe"`
	Valor      core.Centavos `json:"valor_centavos"`
	Ativos     int           `json:"ativos"`
	Percentual float64       `json:"percentual"`
}

// Carteira da entidade (ou entidades) em uma data.
type Carteira struct {
	Total      core.Centavos    `json:"total_centavos"`
	Custo      core.Centavos    `json:"custo_centavos"`
	Classes    []ClasseCarteira `json:"classes"`
	Ativos     []AtivoCarteira  `json:"ativos"`
	Encerrados int              `json:"encerrados"`
	ValorEm    string           `json:"valor_em"`
	// Rentabilidade (XIRR) dos ativos com operações, e CDI e IPCA no mesmo período, ao ano
	Rentabilidade *float64 `json:"rentabilidade_aa"`
	Desde         *string  `json:"rentabilidade_desde"`
	CDI           *float64 `json:"cdi_aa"`
	IPCA          *float64 `json:"ipca_aa"`
}

// historiaMinima para mostrar taxa ao ano: menos que isso, a anualização engana.
const historiaMinima = 30

// indicesCarteira: CDI diário e IPCA mensal, lidos uma vez só quando algum ativo precisa.
type indicesCarteira struct {
	lidos bool
	cdi   []core.TaxaDia
	ipca  map[string]float64
}

func (ix *indicesCarteira) carregar(ctx context.Context, tx pgx.Tx) error {
	if ix.lidos {
		return nil
	}
	ix.lidos, ix.ipca = true, map[string]float64{}
	linhas, err := tx.Query(ctx, "select data, valor::float8 / 100 from indices where serie = 'cdi' order by data")
	if err != nil {
		return err
	}
	for linhas.Next() {
		var d core.TaxaDia
		if err := linhas.Scan(&d.Data, &d.Taxa); err != nil {
			linhas.Close()
			return err
		}
		ix.cdi = append(ix.cdi, d)
	}
	linhas.Close()
	if err := linhas.Err(); err != nil {
		return err
	}
	linhas, err = tx.Query(ctx, "select to_char(data, 'YYYY-MM'), valor::float8 / 100 from indices where serie = 'ipca'")
	if err != nil {
		return err
	}
	defer linhas.Close()
	for linhas.Next() {
		var m string
		var v float64
		if err := linhas.Scan(&m, &v); err != nil {
			return err
		}
		ix.ipca[m] = v
	}
	return linhas.Err()
}

// benchmarks: CDI e IPCA acumulados entre as datas, ao ano. nil sem índice que cubra o período.
func (ix *indicesCarteira) benchmarks(de, ate time.Time) (cdi, ipca *float64) {
	dias := int(ate.Sub(de).Hours() / 24)
	var taxas []float64
	for _, d := range ix.cdi {
		if d.Data.After(de) && !d.Data.After(ate) {
			taxas = append(taxas, d.Taxa)
		}
	}
	if len(ix.cdi) > 0 && !ix.cdi[0].Data.After(de.AddDate(0, 0, 7)) && len(taxas) > 0 {
		v := core.Anualizar(core.Acumular(taxas), dias)
		cdi = &v
	}
	if _, ok := ix.ipca[de.Format("2006-01")]; ok {
		const base = 100_000_000_00
		f := core.ValorNaCurva(core.RendaFixa{Indexador: core.IdxIPCA, Aplicacao: de, Principal: base}, ate, nil, ix.ipca)
		v := core.Anualizar(float64(f)/base-1, dias)
		ipca = &v
	}
	return cdi, ipca
}

type ativoLido struct {
	AtivoCarteira
	saldo, aplicado *int64
	cotacaoCodigo   *string
	indexador       *string
	taxa            *float64
}

// CalcularCarteira: ativo do Open Finance vale o saldo que o banco informa; os outros,
// a posição pelas operações × a última cotação até a data (sem cotação, o custo).
func CalcularCarteira(ctx context.Context, tx pgx.Tx, ents []string, data time.Time) (Carteira, error) {
	c := Carteira{ValorEm: data.Format("2006-01-02"), Classes: []ClasseCarteira{}, Ativos: []AtivoCarteira{}}
	linhas, err := tx.Query(ctx, `select id, entidade_id, codigo, coalesce(nome, codigo), classe::text, subtipo, instituicao,
			emissor, to_char(vencimento, 'YYYY-MM-DD'), origem, encerrado, isento_ir, saldo_centavos, aplicado_centavos,
			cotacao_codigo, indexador, taxa::float8
		from ativos where entidade_id = any($1) order by classe, nome`, ents)
	if err != nil {
		return c, err
	}
	var lidos []ativoLido
	for linhas.Next() {
		var a ativoLido
		if err := linhas.Scan(&a.ID, &a.EntidadeID, &a.Codigo, &a.Nome, &a.Classe, &a.Subtipo, &a.Instituicao, &a.Emissor,
			&a.Vencimento, &a.Origem, &a.Encerrado, &a.Isento, &a.saldo, &a.aplicado, &a.cotacaoCodigo,
			&a.indexador, &a.taxa); err != nil {
			linhas.Close()
			return c, err
		}
		lidos = append(lidos, a)
	}
	linhas.Close()
	if err := linhas.Err(); err != nil {
		return c, err
	}

	porClasse := map[string]*ClasseCarteira{}
	var fluxos []core.Fluxo
	ix := &indicesCarteira{}
	for _, a := range lidos {
		if a.Origem == "pluggy" {
			if a.saldo != nil {
				a.Valor = core.Centavos(*a.saldo)
			}
			if a.aplicado != nil {
				a.Custo = core.Centavos(*a.aplicado)
			}
			// com as movimentações do banco (aportes e resgates), a rentabilidade ao ano
			ops, _, err := operacoesDoAtivo(ctx, tx, a.ID)
			if err != nil {
				return c, err
			}
			if fs := core.FluxosDasOperacoes(ops, a.Valor, data); historicoCompleto(fs, a.Custo, a.Valor) {
				if len(fs) > 1 && int(data.Sub(fs[0].Data).Hours()/24) >= historiaMinima {
					if r, err := core.XIRR(fs); err == nil {
						a.Rentabilidade = &r
					}
				}
				fluxos = append(fluxos, fs...)
			}
		} else {
			ops, evs, err := operacoesDoAtivo(ctx, tx, a.ID)
			if err != nil {
				return c, err
			}
			p, err := core.CalcularPosicao(ops, evs, data)
			if err != nil {
				return c, err
			}
			a.Quantidade, a.PrecoMedio = p.Quantidade.String(), p.PrecoMedio.String()
			a.Custo, a.Proventos, a.Valor = p.Custo, p.Proventos, p.Custo
			a.Encerrado = a.Encerrado || p.Quantidade == 0
			cotado := false
			if a.cotacaoCodigo != nil && p.Quantidade > 0 {
				var preco, em string
				err := tx.QueryRow(ctx, `select preco::text, to_char(data, 'YYYY-MM-DD') from cotacoes
					where codigo = $1 and data <= $2 order by data desc limit 1`, *a.cotacaoCodigo, data).Scan(&preco, &em)
				if err == nil {
					if v, err := core.ParseDec8(preco); err == nil {
						a.Valor = core.Valor(p.Quantidade, v)
						a.Cotacao, a.CotacaoEm = &preco, &em
						cotado = true
					}
				}
			}
			if !cotado && p.Quantidade > 0 && a.indexador != nil && a.taxa != nil {
				if err := ix.carregar(ctx, tx); err != nil {
					return c, err
				}
				a.Valor, a.NaCurva = valorNaCurva(ops, *a.indexador, *a.taxa, p.Quantidade, data, ix), true
			}
			fs := core.FluxosDasOperacoes(ops, a.Valor, data)
			if len(fs) > 1 && int(data.Sub(fs[0].Data).Hours()/24) >= historiaMinima {
				if r, err := core.XIRR(fs); err == nil {
					a.Rentabilidade = &r
				}
			}
			fluxos = append(fluxos, fs...)
		}
		if a.Encerrado && a.Valor == 0 {
			c.Encerrados++
		}
		a.Rendimento = a.Valor - a.Custo
		c.Total += a.Valor
		c.Custo += a.Custo
		if porClasse[a.Classe] == nil {
			porClasse[a.Classe] = &ClasseCarteira{Classe: a.Classe}
		}
		if a.Valor != 0 {
			porClasse[a.Classe].Valor += a.Valor
			porClasse[a.Classe].Ativos++
		}
		c.Ativos = append(c.Ativos, a.AtivoCarteira)
	}
	for i := range c.Ativos {
		if c.Total > 0 {
			c.Ativos[i].Percentual = float64(c.Ativos[i].Valor) / float64(c.Total)
		}
	}
	for _, cl := range porClasse {
		if cl.Ativos == 0 {
			continue
		}
		if c.Total > 0 {
			cl.Percentual = float64(cl.Valor) / float64(c.Total)
		}
		c.Classes = append(c.Classes, *cl)
	}
	sort.Slice(fluxos, func(i, j int) bool { return fluxos[i].Data.Before(fluxos[j].Data) })
	if len(fluxos) > 1 && int(data.Sub(fluxos[0].Data).Hours()/24) >= historiaMinima {
		if r, err := core.XIRR(fluxos); err == nil {
			desde := fluxos[0].Data.Format("2006-01-02")
			c.Rentabilidade, c.Desde = &r, &desde
			if err := ix.carregar(ctx, tx); err != nil {
				return c, err
			}
			c.CDI, c.IPCA = ix.benchmarks(fluxos[0].Data, data)
		}
	}
	sort.Slice(c.Classes, func(i, j int) bool { return c.Classes[i].Valor > c.Classes[j].Valor })
	sort.SliceStable(c.Ativos, func(i, j int) bool { return c.Ativos[i].Valor > c.Ativos[j].Valor })
	return c, nil
}

// historicoCompleto: o banco pode mandar só os últimos meses de movimentos; sem o
// aporte inicial, a taxa sairia absurda. Exige que os aportes cubram 90 % do aplicado
// (ou, sem esse número, metade do saldo).
func historicoCompleto(fs []core.Fluxo, aplicado, saldo core.Centavos) bool {
	var aportes core.Centavos
	for _, f := range fs {
		if f.Valor < 0 {
			aportes -= f.Valor
		}
	}
	if aportes == 0 {
		return false
	}
	if aplicado > 0 {
		return aportes*10 >= aplicado*9
	}
	return aportes*2 >= saldo
}

// valorNaCurva da renda fixa sem cotação: cada compra rende pela taxa contratada desde a
// data dela; vendas e amortizações reduzem na proporção da quantidade que resta.
func valorNaCurva(ops []core.Operacao, indexador string, taxa float64, resta core.Dec8, data time.Time, ix *indicesCarteira) core.Centavos {
	var total core.Centavos
	var comprada core.Dec8
	for _, o := range ops {
		if o.Tipo != core.OpCompra || o.Data.After(data) {
			continue
		}
		comprada += o.Quantidade
		total += core.ValorNaCurva(core.RendaFixa{Indexador: indexador, Taxa: taxa, Aplicacao: o.Data,
			Principal: core.Valor(o.Quantidade, o.Preco) + o.Taxas}, data, ix.cdi, ix.ipca)
	}
	if comprada == 0 {
		return 0
	}
	return core.Proporcao(total, resta, comprada)
}

func operacoesDoAtivo(ctx context.Context, tx pgx.Tx, ativoID string) ([]core.Operacao, []core.Evento, error) {
	linhas, err := tx.Query(ctx, `select data, tipo::text, quantidade::text, preco::text, valor_centavos, taxas_centavos,
			ir_retido_centavos from operacoes where ativo_id = $1 order by data, criada_em`, ativoID)
	if err != nil {
		return nil, nil, err
	}
	var ops []core.Operacao
	for linhas.Next() {
		var o core.Operacao
		var q, p string
		var valor, taxas, ir int64
		if err := linhas.Scan(&o.Data, &o.Tipo, &q, &p, &valor, &taxas, &ir); err != nil {
			linhas.Close()
			return nil, nil, err
		}
		o.Quantidade, _ = core.ParseDec8(q)
		o.Preco, _ = core.ParseDec8(p)
		o.Valor, o.Taxas, o.IRRetido = core.Centavos(valor), core.Centavos(taxas), core.Centavos(ir)
		ops = append(ops, o)
	}
	linhas.Close()
	if err := linhas.Err(); err != nil {
		return nil, nil, err
	}
	linhas, err = tx.Query(ctx, `select data, tipo::text, fator::text, custo_unitario::text from eventos_corporativos
		where ativo_id = $1 order by data, criado_em`, ativoID)
	if err != nil {
		return nil, nil, err
	}
	defer linhas.Close()
	var evs []core.Evento
	for linhas.Next() {
		var e core.Evento
		var f, cu string
		if err := linhas.Scan(&e.Data, &e.Tipo, &f, &cu); err != nil {
			return nil, nil, err
		}
		e.Fator, _ = core.ParseDec8(f)
		e.CustoUnitario, _ = core.ParseDec8(cu)
		evs = append(evs, e)
	}
	return ops, evs, linhas.Err()
}
