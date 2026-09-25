package core

import (
	"errors"
	"sort"
	"time"
)

// Tipos de operação de um ativo.
const (
	OpCompra      = "compra"
	OpVenda       = "venda"
	OpProvento    = "provento"    // dividendo, JCP, rendimento de FII: dinheiro que sai do ativo
	OpJuros       = "juros"       // juros pagos por renda fixa
	OpAmortizacao = "amortizacao" // devolução de parte do principal (reduz o custo)
)

// Tipos de evento corporativo.
const (
	EvDesdobramento = "desdobramento" // quantidade × fator (ex.: 1 vira 2 → fator 2)
	EvGrupamento    = "grupamento"    // quantidade × fator (ex.: 10 viram 1 → fator 0,1)
	EvBonificacao   = "bonificacao"   // ganha quantidade × (fator − 1) ações ao custo informado
	EvAjuste        = "ajuste"        // soma "fator" à quantidade (é como a B3 informa desdobro e bonificação)
)

// ErrVendaSemPosicao: venda de mais do que se tem (venda a descoberto não é suportada).
var ErrVendaSemPosicao = errors.New("venda maior que a posição")

// Operacao de um ativo. Valor é usado em provento, juros e amortização.
type Operacao struct {
	Data       time.Time
	Tipo       string
	Quantidade Dec8
	Preco      Dec8
	Valor      Centavos
	Taxas      Centavos
	IRRetido   Centavos
}

// Evento corporativo.
type Evento struct {
	Data          time.Time
	Tipo          string
	Fator         Dec8
	CustoUnitario Dec8 // bonificação: custo atribuído a cada ação nova
}

// Venda realizada: o que entra na apuração do IR.
type Venda struct {
	Data       time.Time
	Quantidade Dec8
	Valor      Centavos // valor da venda menos as taxas
	Custo      Centavos // custo médio da quantidade vendida (com as taxas da compra)
	Ganho      Centavos
	DayTrade   bool
	IRRetido   Centavos // "dedo-duro" retido na fonte
}

// Posicao de um ativo numa data.
type Posicao struct {
	Quantidade Dec8
	Custo      Centavos // custo total da posição
	PrecoMedio Dec8
	Proventos  Centavos // recebidos (líquidos do IR retido)
	Vendas     []Venda
}

type movimento struct {
	data   time.Time
	evento *Evento
	ops    []Operacao
}

// CalcularPosicao aplica operações e eventos em ordem de data. No mesmo dia, o evento
// vem antes das operações, e compra e venda do mesmo dia formam day trade na
// quantidade que se casa (preços médios do dia), como a Receita apura; o resto do dia
// segue para a posição normal (compras primeiro).
func CalcularPosicao(ops []Operacao, eventos []Evento, ate time.Time) (Posicao, error) {
	porDia := map[string]*movimento{}
	var dias []string
	pegar := func(d time.Time) *movimento {
		k := d.Format("2006-01-02")
		if porDia[k] == nil {
			porDia[k] = &movimento{data: d}
			dias = append(dias, k)
		}
		return porDia[k]
	}
	fim := truncarDia(ate)
	for _, o := range ops {
		if !truncarDia(o.Data).After(fim) {
			m := pegar(truncarDia(o.Data))
			m.ops = append(m.ops, o)
		}
	}
	var p Posicao
	evPorDia := map[string][]Evento{}
	for _, e := range eventos {
		if !truncarDia(e.Data).After(fim) {
			k := truncarDia(e.Data).Format("2006-01-02")
			evPorDia[k] = append(evPorDia[k], e)
			pegar(truncarDia(e.Data))
		}
	}
	sort.Strings(dias)

	for _, dia := range dias {
		for _, e := range evPorDia[dia] {
			aplicarEvento(&p, e)
		}
		if err := aplicarDia(&p, porDia[dia]); err != nil {
			return p, err
		}
	}
	p.PrecoMedio = PrecoMedio(p.Custo, p.Quantidade)
	return p, nil
}

func aplicarEvento(p *Posicao, e Evento) {
	switch e.Tipo {
	case EvDesdobramento, EvGrupamento:
		p.Quantidade = MultDec8(p.Quantidade, e.Fator)
	case EvBonificacao:
		novas := MultDec8(p.Quantidade, e.Fator-Um)
		p.Quantidade += novas
		p.Custo += Valor(novas, e.CustoUnitario)
	case EvAjuste:
		p.Quantidade += e.Fator
		if e.Fator > 0 {
			p.Custo += Valor(e.Fator, e.CustoUnitario)
		}
	}
}

func aplicarDia(p *Posicao, m *movimento) error {
	var comprasQ, vendasQ Dec8
	var comprasV, vendasV, comprasT, vendasT, vendasIR Centavos
	for _, o := range m.ops {
		switch o.Tipo {
		case OpCompra:
			comprasQ += o.Quantidade
			comprasV += Valor(o.Quantidade, o.Preco)
			comprasT += o.Taxas
		case OpVenda:
			vendasQ += o.Quantidade
			vendasV += Valor(o.Quantidade, o.Preco)
			vendasT += o.Taxas
			vendasIR += o.IRRetido
		case OpProvento, OpJuros:
			p.Proventos += o.Valor - o.IRRetido
		case OpAmortizacao:
			p.Custo -= min(o.Valor, p.Custo)
		}
	}

	// day trade: a menor das duas quantidades, pelos preços médios do dia
	dt := min(comprasQ, vendasQ)
	if dt > 0 {
		custoDT := Proporcao(comprasV+comprasT, dt, comprasQ)
		valorDT := Proporcao(vendasV-vendasT, dt, vendasQ)
		irDT := Proporcao(vendasIR, dt, vendasQ)
		p.Vendas = append(p.Vendas, Venda{Data: m.data, Quantidade: dt, Valor: valorDT, Custo: custoDT,
			Ganho: valorDT - custoDT, DayTrade: true, IRRetido: irDT})
		comprasV, comprasT = comprasV-Proporcao(comprasV, dt, comprasQ), comprasT-Proporcao(comprasT, dt, comprasQ)
		sobraVenda := vendasV - vendasT - valorDT
		vendasIR -= irDT
		comprasQ, vendasQ = comprasQ-dt, vendasQ-dt
		vendasV, vendasT = sobraVenda, 0
	}

	// o que sobrou: compras entram no custo; vendas saem pelo preço médio
	if comprasQ > 0 {
		p.Quantidade += comprasQ
		p.Custo += comprasV + comprasT
	}
	if vendasQ > 0 {
		if vendasQ > p.Quantidade {
			return ErrVendaSemPosicao
		}
		custo := Proporcao(p.Custo, vendasQ, p.Quantidade)
		valor := vendasV - vendasT
		p.Vendas = append(p.Vendas, Venda{Data: m.data, Quantidade: vendasQ, Valor: valor, Custo: custo,
			Ganho: valor - custo, IRRetido: vendasIR})
		p.Quantidade -= vendasQ
		p.Custo -= custo
		if p.Quantidade == 0 {
			p.Custo = 0
		}
	}
	return nil
}
