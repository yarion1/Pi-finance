package financeiro

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yarion1/pi-finance/internal/core"
)

func itoa(n int) string { return strconv.Itoa(n) }

var fusoSP = func() *time.Location {
	l, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		return time.FixedZone("BRT", -3*3600)
	}
	return l
}()

// Hoje em São Paulo, só a data.
func Hoje() time.Time { return dia(time.Now().In(fusoSP)) }

// Agora em São Paulo (com a hora: o resumo da semana sai no domingo à noite).
func Agora() time.Time { return time.Now().In(fusoSP) }

func fmtValor(v any) string { return fmt.Sprint(v) }

func fimDoMes(t time.Time) time.Time {
	return core.DiaNoMes(t.Year(), t.Month(), 31)
}

// ---------------------------------------------------------------------------
// Comprometido nos próximos meses
// ---------------------------------------------------------------------------

// MesComprometido soma o que já está contratado para o mês (valores positivos).
type MesComprometido struct {
	Mes          string `json:"mes"`
	Parcelas     int64  `json:"parcelas_centavos"`
	Dividas      int64  `json:"dividas_centavos"`
	Recorrentes  int64  `json:"recorrentes_centavos"`
	Compromissos int64  `json:"compromissos_centavos"`
	Total        int64  `json:"total_centavos"`
}

// Comprometido: parcelas, financiamentos, contas fixas e contas avulsas a pagar,
// do mês atual (o que falta) aos próximos.
func Comprometido(ctx context.Context, tx pgx.Tx, ents []string, hoje time.Time, meses int) ([]MesComprometido, error) {
	hoje = dia(hoje)
	out := make([]MesComprometido, meses)
	indice := map[string]int{}
	for i := range out {
		m := core.DiaNoMes(hoje.Year(), hoje.Month()+time.Month(i), 1).Format("2006-01")
		out[i].Mes, indice[m] = m, i
	}
	fim := fimDoMes(core.DiaNoMes(hoje.Year(), hoje.Month()+time.Month(meses-1), 1))
	somar := func(mes string, campo func(*MesComprometido) *int64, v int64) {
		if i, ok := indice[mes]; ok {
			*campo(&out[i]) += v
		}
	}

	parcelas, err := ParcelasFuturas(ctx, tx, ents, "", hoje)
	if err != nil {
		return nil, err
	}
	for _, p := range parcelas {
		if p.Valor < 0 {
			somar(p.Mes, func(m *MesComprometido) *int64 { return &m.Parcelas }, -p.Valor)
		}
	}
	dividas, err := Dividas(ctx, tx, ents, hoje)
	if err != nil {
		return nil, err
	}
	for _, d := range dividas {
		for _, p := range d.tabela {
			if p.Vencimento.After(hoje) && !p.Vencimento.After(fim) {
				somar(p.Vencimento.Format("2006-01"), func(m *MesComprometido) *int64 { return &m.Dividas }, int64(p.Prestacao))
			}
		}
	}
	recs, err := Recorrencias(ctx, tx, ents, hoje)
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		if !r.Ativa || r.Valor >= 0 {
			continue
		}
		for _, d := range r.Ocorrencias(hoje, fim) {
			somar(d.Format("2006-01"), func(m *MesComprometido) *int64 { return &m.Recorrentes }, -r.Valor)
		}
	}
	linhas, err := tx.Query(ctx, `select vencimento, valor_centavos from compromissos
		where entidade_id = any($1) and pago_em is null and valor_centavos < 0 and vencimento <= $2`, ents, fim)
	if err != nil {
		return nil, err
	}
	for linhas.Next() {
		var v time.Time
		var valor int64
		if err := linhas.Scan(&v, &valor); err != nil {
			linhas.Close()
			return nil, err
		}
		mes := v.Format("2006-01")
		if v.Before(hoje) {
			mes = hoje.Format("2006-01") // atrasado: ainda é para pagar agora
		}
		somar(mes, func(m *MesComprometido) *int64 { return &m.Compromissos }, -valor)
	}
	linhas.Close()
	for i := range out {
		out[i].Total = out[i].Parcelas + out[i].Dividas + out[i].Recorrentes + out[i].Compromissos
	}
	return out, linhas.Err()
}

// ---------------------------------------------------------------------------
// Custo de vida
// ---------------------------------------------------------------------------

// CustoDeVida médio dos últimos 3, 6 e 12 meses fechados (gasto − estornos, sem
// transferências). Meses antes da primeira transação não entram na média.
type CustoDeVida struct {
	Meses3  int64 `json:"medio_3_meses_centavos"`
	Meses6  int64 `json:"medio_6_meses_centavos"`
	Meses12 int64 `json:"medio_12_meses_centavos"`
}

func CalcularCustoDeVida(ctx context.Context, tx pgx.Tx, ents []string, hoje time.Time) (CustoDeVida, error) {
	var c CustoDeVida
	inicioMes := core.DiaNoMes(hoje.Year(), hoje.Month(), 1)
	de := core.DiaNoMes(hoje.Year(), hoje.Month()-12, 1)
	var primeira *time.Time
	if err := tx.QueryRow(ctx, "select min(data) from transacoes where entidade_id = any($1)", ents).Scan(&primeira); err != nil || primeira == nil {
		return c, err
	}
	linhas, err := tx.Query(ctx, `select to_char(data, 'YYYY-MM'), coalesce(-sum(valor_centavos), 0) from transacoes
		where entidade_id = any($1) and tipo in ('gasto', 'estorno') and moeda = 'BRL' and data >= $2 and data < $3
		group by 1`, ents, de, inicioMes)
	if err != nil {
		return c, err
	}
	porMes := map[string]int64{}
	for linhas.Next() {
		var m string
		var v int64
		if err := linhas.Scan(&m, &v); err != nil {
			linhas.Close()
			return c, err
		}
		porMes[m] = v
	}
	linhas.Close()
	primeiroMes := core.DiaNoMes(primeira.Year(), primeira.Month(), 1)
	media := func(n int) int64 {
		var vals []core.Centavos
		for i := 1; i <= n; i++ {
			m := core.DiaNoMes(hoje.Year(), hoje.Month()-time.Month(i), 1)
			if m.Before(primeiroMes) {
				break
			}
			vals = append(vals, core.Centavos(porMes[m.Format("2006-01")]))
		}
		return int64(core.Media(vals))
	}
	c.Meses3, c.Meses6, c.Meses12 = media(3), media(6), media(12)
	return c, linhas.Err()
}

// ---------------------------------------------------------------------------
// Patrimônio
// ---------------------------------------------------------------------------

// PontoPatrimonio na série mensal.
type PontoPatrimonio struct {
	Data     string `json:"data"`
	Ativos   int64  `json:"ativos_centavos"`
	Passivos int64  `json:"passivos_centavos"`
	Liquido  int64  `json:"liquido_centavos"`
}

type contaSaldo struct {
	id, tipo, moeda string
}

type bemValor struct {
	valor  int64
	criado time.Time
}

func patrimonioEm(ctx context.Context, tx pgx.Tx, contas []contaSaldo, bens []bemValor, dividas []Divida, data time.Time) (PontoPatrimonio, error) {
	p := PontoPatrimonio{Data: data.Format(formatoData)}
	for _, c := range contas {
		if c.moeda != "BRL" {
			continue
		}
		var saldo *int64
		if err := tx.QueryRow(ctx, "select app_saldo_conta($1, $2)", c.id, data).Scan(&saldo); err != nil {
			return p, err
		}
		if saldo == nil {
			continue
		}
		if *saldo >= 0 {
			p.Ativos += *saldo
		} else {
			p.Passivos -= *saldo
		}
	}
	for _, b := range bens {
		if !b.criado.After(data) {
			p.Ativos += b.valor
		}
	}
	for _, d := range dividas {
		p.Passivos += d.SaldoEm(data)
	}
	p.Liquido = p.Ativos - p.Passivos
	return p, nil
}

// Patrimonio: posição de hoje e o fim de cada um dos últimos 12 meses.
type Patrimonio struct {
	Hoje          PontoPatrimonio   `json:"hoje"`
	VariacaoMes   int64             `json:"variacao_mes_centavos"`
	VariacaoAno   int64             `json:"variacao_ano_centavos"`
	Serie         []PontoPatrimonio `json:"serie"`
	Contas        int64             `json:"contas_centavos"`
	Investimentos int64             `json:"investimentos_centavos"`
	Bens          int64             `json:"bens_centavos"`
	Cartoes       int64             `json:"cartoes_centavos"`
	Negativas     int64             `json:"contas_negativas_centavos"` // cheque especial: conta com saldo abaixo de zero
	Dividas       int64             `json:"dividas_centavos"`
	// empréstimos e financiamentos que o banco informa pelo Open Finance
	EmprestimosBanco []EmprestimoBanco `json:"emprestimos_banco"`
	DividasBanco     int64             `json:"dividas_banco_centavos"`
}

// EmprestimoBanco: contrato de crédito informado pelo banco.
type EmprestimoBanco struct {
	ID                string  `json:"id"`
	Nome              string  `json:"nome"`
	Modalidade        string  `json:"modalidade"`
	Instituicao       *string `json:"instituicao"`
	ValorContratado   *int64  `json:"valor_contratado_centavos"`
	SaldoDevedor      *int64  `json:"saldo_devedor_centavos"`
	ParcelasTotal     *int    `json:"parcelas_total"`
	ParcelasPagas     *int    `json:"parcelas_pagas"`
	ParcelasRestantes *int    `json:"parcelas_restantes"`
	ParcelasAtrasadas *int    `json:"parcelas_atrasadas"`
	Taxa              *string `json:"taxa"`
	TaxaPeriodicidade *string `json:"taxa_periodicidade"`
	CET               *string `json:"cet"`
	Sistema           *string `json:"sistema"`
	VencimentoFinal   *string `json:"vencimento_final"`
	ContaNoPassivo    bool    `json:"conta_no_passivo"` // cheque especial e rotativo já estão no saldo da conta/cartão
}

// EmprestimosDoBanco das entidades, maiores saldos primeiro.
func EmprestimosDoBanco(ctx context.Context, tx pgx.Tx, ents []string) ([]EmprestimoBanco, error) {
	linhas, err := tx.Query(ctx, `select e.id, e.nome, e.modalidade, i.instituicao, e.valor_contratado_centavos,
			e.saldo_devedor_centavos, e.parcelas_total, e.parcelas_pagas, e.parcelas_restantes, e.parcelas_atrasadas,
			e.taxa::text, e.taxa_periodicidade, e.cet::text, e.sistema, to_char(e.vencimento_final, 'YYYY-MM-DD'),
			e.modalidade in ('LOAN', 'FINANCING')
		from emprestimos_banco e join itens_pluggy i on i.id = e.item_id
		where e.entidade_id = any($1) and e.moeda = 'BRL'
		order by coalesce(e.saldo_devedor_centavos, 0) desc`, ents)
	if err != nil {
		return nil, err
	}
	lista, err := pgx.CollectRows(linhas, pgx.RowToStructByPos[EmprestimoBanco])
	if lista == nil {
		lista = []EmprestimoBanco{}
	}
	return lista, err
}

func CalcularPatrimonio(ctx context.Context, tx pgx.Tx, ents []string, hoje time.Time, comSerie bool) (Patrimonio, error) {
	hoje = dia(hoje)
	var p Patrimonio
	linhas, err := tx.Query(ctx, "select id, tipo::text, moeda from contas where entidade_id = any($1) and not arquivada", ents)
	if err != nil {
		return p, err
	}
	var contas []contaSaldo
	for linhas.Next() {
		var c contaSaldo
		if err := linhas.Scan(&c.id, &c.tipo, &c.moeda); err != nil {
			linhas.Close()
			return p, err
		}
		contas = append(contas, c)
	}
	linhas.Close()

	linhas, err = tx.Query(ctx, "select valor_centavos, criado_em::date from bens where entidade_id = any($1)", ents)
	if err != nil {
		return p, err
	}
	var bens []bemValor
	for linhas.Next() {
		var b bemValor
		if err := linhas.Scan(&b.valor, &b.criado); err != nil {
			linhas.Close()
			return p, err
		}
		bens = append(bens, b)
		p.Bens += b.valor
	}
	linhas.Close()

	dividas, err := Dividas(ctx, tx, ents, hoje)
	if err != nil {
		return p, err
	}
	for _, d := range dividas {
		p.Dividas += d.SaldoDevedor
	}
	if p.EmprestimosBanco, err = EmprestimosDoBanco(ctx, tx, ents); err != nil {
		return p, err
	}
	for _, e := range p.EmprestimosBanco {
		if e.ContaNoPassivo && e.SaldoDevedor != nil {
			p.DividasBanco += *e.SaldoDevedor
		}
	}
	// cartão: dívida inclui as parcelas futuras já lançadas (limite usado)
	for _, c := range contas {
		if c.moeda != "BRL" {
			continue
		}
		var saldo *int64
		ate := &hoje
		if c.tipo == "cartao" {
			ate = nil
		}
		if err := tx.QueryRow(ctx, "select app_saldo_conta($1, $2)", c.id, ate).Scan(&saldo); err != nil {
			return p, err
		}
		if saldo == nil {
			continue
		}
		switch {
		case c.tipo == "cartao":
			p.Cartoes -= min(*saldo, 0)
			p.Contas += max(*saldo, 0) // crédito na fatura
		case *saldo < 0:
			p.Negativas -= *saldo
		case c.tipo == "investimento":
			p.Investimentos += *saldo
		default:
			p.Contas += *saldo
		}
	}
	// a carteira de investimentos (ativos) entra além das contas do tipo investimento
	carteira, err := CalcularCarteira(ctx, tx, ents, hoje)
	if err != nil {
		return p, err
	}
	p.Investimentos += int64(carteira.Total)
	p.Hoje = PontoPatrimonio{Data: hoje.Format(formatoData), Ativos: p.Contas + p.Investimentos + p.Bens, Passivos: p.Cartoes + p.Negativas + p.Dividas + p.DividasBanco}
	p.Hoje.Liquido = p.Hoje.Ativos - p.Hoje.Passivos

	fimMesPassado := core.DiaNoMes(hoje.Year(), hoje.Month(), 1).AddDate(0, 0, -1)
	fimAnoPassado := time.Date(hoje.Year()-1, 12, 31, 0, 0, 0, 0, time.UTC)
	for _, ref := range []struct {
		data time.Time
		var_ *int64
	}{{fimMesPassado, &p.VariacaoMes}, {fimAnoPassado, &p.VariacaoAno}} {
		pt, err := patrimonioEm(ctx, tx, contas, bens, dividas, ref.data)
		if err != nil {
			return p, err
		}
		// sem histórico dos empréstimos do banco: o saldo de hoje vale para trás também
		pt.Passivos, pt.Liquido = pt.Passivos+p.DividasBanco, pt.Liquido-p.DividasBanco
		*ref.var_ = p.Hoje.Liquido - pt.Liquido
	}
	if comSerie {
		for i := 12; i >= 1; i-- {
			pt, err := patrimonioEm(ctx, tx, contas, bens, dividas, fimDoMes(core.DiaNoMes(hoje.Year(), hoje.Month()-time.Month(i), 1)))
			if err != nil {
				return p, err
			}
			pt.Passivos, pt.Liquido = pt.Passivos+p.DividasBanco, pt.Liquido-p.DividasBanco
			p.Serie = append(p.Serie, pt)
		}
		p.Serie = append(p.Serie, p.Hoje)
	}
	return p, nil
}

// ---------------------------------------------------------------------------
// Agenda e saldo projetado
// ---------------------------------------------------------------------------

// ItemAgenda é uma conta a pagar ou a receber.
type ItemAgenda struct {
	Data      string  `json:"data"`
	Descricao string  `json:"descricao"`
	Valor     int64   `json:"valor_centavos"`
	Origem    string  `json:"origem"` // recorrencia, fatura, divida, compromisso, lancamento
	ID        string  `json:"id"`
	ContaID   *string `json:"conta_id"`
	Conta     *string `json:"conta"`
	Pago      bool    `json:"pago"`
	Atrasado  bool    `json:"atrasado"`
	NoCartao  bool    `json:"no_cartao"` // cai na fatura; não mexe no saldo da conta corrente
}

// SaldoDia do saldo projetado.
type SaldoDia struct {
	Data  string `json:"data"`
	Saldo int64  `json:"saldo_centavos"`
}

// Agenda dos próximos dias e o saldo projetado dia a dia das contas do dia a dia
// (corrente, poupança, carteira, dinheiro, benefício).
type Agenda struct {
	Itens        []ItemAgenda `json:"itens"`
	SaldoInicial int64        `json:"saldo_inicial_centavos"`
	Saldos       []SaldoDia   `json:"saldos"`
	SaldoFinal   int64        `json:"saldo_final_centavos"`
	MenorSaldo   SaldoDia     `json:"menor_saldo"`
}

func CalcularAgenda(ctx context.Context, tx pgx.Tx, ents []string, hoje time.Time, dias int) (Agenda, error) {
	hoje = dia(hoje)
	ate := hoje.AddDate(0, 0, dias)
	var ag Agenda

	// saldo de hoje das contas do dia a dia
	if err := tx.QueryRow(ctx, `select coalesce(sum(app_saldo_conta(id, $2)), 0) from contas
		where entidade_id = any($1) and not arquivada and moeda = 'BRL'
		  and tipo in ('corrente', 'poupanca', 'carteira', 'dinheiro', 'beneficio')`, ents, hoje).Scan(&ag.SaldoInicial); err != nil {
		return ag, err
	}

	recs, err := Recorrencias(ctx, tx, ents, hoje)
	if err != nil {
		return ag, err
	}
	for _, r := range recs {
		if !r.Ativa {
			continue
		}
		for _, d := range r.Ocorrencias(hoje, ate) {
			ag.Itens = append(ag.Itens, ItemAgenda{Data: d.Format(formatoData), Descricao: r.Descricao, Valor: r.Valor,
				Origem: "recorrencia", ID: r.ID, ContaID: r.ContaID, Conta: r.Conta, NoCartao: r.ContaCartao})
		}
	}

	cs, err := cartoes(ctx, tx, ents, "")
	if err != nil {
		return ag, err
	}
	for _, c := range cs {
		faturas, err := Faturas(ctx, tx, c.id, hoje)
		if err != nil {
			return ag, err
		}
		for _, f := range faturas {
			venc, _ := time.Parse(formatoData, f.Vencimento)
			total := f.Compras + f.Projetado
			if venc.Before(hoje) || venc.After(ate) || total <= 0 {
				continue
			}
			id, nome := c.id, c.nome
			ag.Itens = append(ag.Itens, ItemAgenda{Data: f.Vencimento, Descricao: "Fatura " + c.nome, Valor: -total,
				Origem: "fatura", ID: c.id + ":" + f.Vencimento, ContaID: &id, Conta: &nome, Pago: f.Paga})
		}
	}

	dividas, err := Dividas(ctx, tx, ents, hoje)
	if err != nil {
		return ag, err
	}
	for _, d := range dividas {
		for _, p := range d.tabela {
			if p.Vencimento.After(hoje) && !p.Vencimento.After(ate) {
				ag.Itens = append(ag.Itens, ItemAgenda{Data: p.Vencimento.Format(formatoData),
					Descricao: fmt.Sprintf("%s (%d/%d)", d.Nome, p.Numero, len(d.tabela)), Valor: -int64(p.Prestacao),
					Origem: "divida", ID: d.ID, ContaID: d.ContaID})
			}
		}
	}

	linhas, err := tx.Query(ctx, `select k.id, k.descricao, k.valor_centavos, k.vencimento, k.pago_em, k.conta_id, c.nome
		from compromissos k left join contas c on c.id = k.conta_id
		where k.entidade_id = any($1) and k.vencimento <= $2 and (k.pago_em is null or k.vencimento >= $3)`, ents, ate, hoje)
	if err != nil {
		return ag, err
	}
	for linhas.Next() {
		var it ItemAgenda
		var venc time.Time
		var pago *time.Time
		if err := linhas.Scan(&it.ID, &it.Descricao, &it.Valor, &venc, &pago, &it.ContaID, &it.Conta); err != nil {
			linhas.Close()
			return ag, err
		}
		it.Origem, it.Pago = "compromisso", pago != nil
		it.Atrasado = !it.Pago && venc.Before(hoje)
		it.Data = venc.Format(formatoData)
		if it.Atrasado {
			it.Data = hoje.Format(formatoData)
		}
		ag.Itens = append(ag.Itens, it)
	}
	linhas.Close()

	// lançamentos com data futura nas contas do dia a dia (ex.: boleto parcelado)
	linhas, err = tx.Query(ctx, `select t.id, t.descricao, t.valor_centavos, t.data, t.conta_id, c.nome from transacoes t
		join contas c on c.id = t.conta_id
		where t.entidade_id = any($1) and t.data > $2 and t.data <= $3 and c.tipo <> 'cartao' and t.tipo <> 'transferencia'`, ents, hoje, ate)
	if err != nil {
		return ag, err
	}
	for linhas.Next() {
		var it ItemAgenda
		var d time.Time
		if err := linhas.Scan(&it.ID, &it.Descricao, &it.Valor, &d, &it.ContaID, &it.Conta); err != nil {
			linhas.Close()
			return ag, err
		}
		it.Data, it.Origem = d.Format(formatoData), "lancamento"
		ag.Itens = append(ag.Itens, it)
	}
	linhas.Close()
	if err := linhas.Err(); err != nil {
		return ag, err
	}

	// calendário fiscal dos CNPJs: DAS do mês, DASN-SIMEI e DEFIS
	fiscais, err := itensFiscais(ctx, tx, ents, hoje, ate)
	if err != nil {
		return ag, err
	}
	ag.Itens = append(ag.Itens, fiscais...)

	sort.SliceStable(ag.Itens, func(i, j int) bool { return ag.Itens[i].Data < ag.Itens[j].Data })

	efeito := map[string]int64{}
	for _, it := range ag.Itens {
		if !it.Pago && !it.NoCartao {
			efeito[it.Data] += it.Valor
		}
	}
	saldo := ag.SaldoInicial
	ag.MenorSaldo = SaldoDia{Data: hoje.Format(formatoData), Saldo: saldo}
	for d := hoje; !d.After(ate); d = d.AddDate(0, 0, 1) {
		k := d.Format(formatoData)
		saldo += efeito[k]
		ag.Saldos = append(ag.Saldos, SaldoDia{Data: k, Saldo: saldo})
		if saldo < ag.MenorSaldo.Saldo {
			ag.MenorSaldo = SaldoDia{Data: k, Saldo: saldo}
		}
	}
	ag.SaldoFinal = saldo
	if ag.Itens == nil {
		ag.Itens = []ItemAgenda{}
	}
	return ag, nil
}
