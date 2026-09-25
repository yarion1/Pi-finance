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
}

type ativoLido struct {
	AtivoCarteira
	saldo, aplicado *int64
	cotacaoCodigo   *string
}

// CalcularCarteira: ativo do Open Finance vale o saldo que o banco informa; os outros,
// a posição pelas operações × a última cotação até a data (sem cotação, o custo).
func CalcularCarteira(ctx context.Context, tx pgx.Tx, ents []string, data time.Time) (Carteira, error) {
	c := Carteira{ValorEm: data.Format("2006-01-02"), Classes: []ClasseCarteira{}, Ativos: []AtivoCarteira{}}
	linhas, err := tx.Query(ctx, `select id, entidade_id, codigo, coalesce(nome, codigo), classe::text, subtipo, instituicao,
			emissor, to_char(vencimento, 'YYYY-MM-DD'), origem, encerrado, isento_ir, saldo_centavos, aplicado_centavos,
			cotacao_codigo
		from ativos where entidade_id = any($1) order by classe, nome`, ents)
	if err != nil {
		return c, err
	}
	var lidos []ativoLido
	for linhas.Next() {
		var a ativoLido
		if err := linhas.Scan(&a.ID, &a.EntidadeID, &a.Codigo, &a.Nome, &a.Classe, &a.Subtipo, &a.Instituicao, &a.Emissor,
			&a.Vencimento, &a.Origem, &a.Encerrado, &a.Isento, &a.saldo, &a.aplicado, &a.cotacaoCodigo); err != nil {
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
	for _, a := range lidos {
		if a.Origem == "pluggy" {
			if a.saldo != nil {
				a.Valor = core.Centavos(*a.saldo)
			}
			if a.aplicado != nil {
				a.Custo = core.Centavos(*a.aplicado)
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
			if a.cotacaoCodigo != nil && p.Quantidade > 0 {
				var preco, em string
				err := tx.QueryRow(ctx, `select preco::text, to_char(data, 'YYYY-MM-DD') from cotacoes
					where codigo = $1 and data <= $2 order by data desc limit 1`, *a.cotacaoCodigo, data).Scan(&preco, &em)
				if err == nil {
					if v, err := core.ParseDec8(preco); err == nil {
						a.Valor = core.Valor(p.Quantidade, v)
						a.Cotacao, a.CotacaoEm = &preco, &em
					}
				}
			}
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
	sort.Slice(c.Classes, func(i, j int) bool { return c.Classes[i].Valor > c.Classes[j].Valor })
	sort.SliceStable(c.Ativos, func(i, j int) bool { return c.Ativos[i].Valor > c.Ativos[j].Valor })
	return c, nil
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
