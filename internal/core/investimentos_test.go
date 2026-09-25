package core

import (
	"math"
	"testing"
	"time"
)

func d8(s string) Dec8 {
	v, err := ParseDec8(s)
	if err != nil {
		panic(s)
	}
	return v
}

func data(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(s)
	}
	return t
}

func TestDec8(t *testing.T) {
	casos := map[string]string{"10": "10", "10.5": "10.5", "0,00012345": "0.00012345", "-3.2": "-3.2", ".5": "0.5", "+7": "7", "1.00000000": "1"}
	for entrada, saida := range casos {
		v, err := ParseDec8(entrada)
		if err != nil || v.String() != saida {
			t.Errorf("%q → %v %v (esperado %s)", entrada, v, err, saida)
		}
	}
	for _, ruim := range []string{"abc", "1.123456789", "1.2.3", "12345678901", "1e3"} {
		if _, err := ParseDec8(ruim); err == nil {
			t.Errorf("%q deveria falhar", ruim)
		}
	}
	if Valor(d8("100"), d8("10.555")) != 105550 || Valor(d8("3"), d8("0.333")) != 100 || Valor(d8("-1"), d8("0.005")) != -1 {
		t.Error("Valor")
	}
	if PrecoMedio(158250, d8("150")) != d8("10.55") || PrecoMedio(100, 0) != 0 {
		t.Errorf("PrecoMedio %v", PrecoMedio(158250, d8("150")))
	}
	if Proporcao(158250, d8("60"), d8("150")) != 63300 || Proporcao(1, 1, 0) != 0 {
		t.Error("Proporcao")
	}
	if MultDec8(d8("100"), d8("0.1")) != d8("10") || MultDec8(d8("3"), d8("1.5")) != d8("4.5") {
		t.Error("MultDec8")
	}
}

// Critério de aceite da fase 4: a carteira de teste bate com a planilha de referência
// (testdata/referencia_carteira.py) até 0,01 p.p.
func TestCarteiraDeReferencia(t *testing.T) {
	ops := []Operacao{
		{Data: data("2025-01-02"), Tipo: OpCompra, Quantidade: d8("100"), Preco: d8("10"), Taxas: 500},
		{Data: data("2025-04-15"), Tipo: OpCompra, Quantidade: d8("50"), Preco: d8("11.5"), Taxas: 250},
		{Data: data("2025-08-20"), Tipo: OpProvento, Valor: 3000},
		{Data: data("2025-10-10"), Tipo: OpVenda, Quantidade: d8("60"), Preco: d8("12.5"), Taxas: 300},
	}
	fechamento := map[string]string{"2025-01-02": "10", "2025-03-31": "11", "2025-04-15": "11.4",
		"2025-06-30": "12", "2025-08-20": "11.8", "2025-10-10": "12.4", "2025-12-31": "13"}
	var pontos []PontoCarteira
	for _, dia := range []string{"2025-01-02", "2025-03-31", "2025-04-15", "2025-06-30", "2025-08-20", "2025-10-10", "2025-12-31"} {
		p, err := CalcularPosicao(ops, nil, data(dia))
		if err != nil {
			t.Fatal(err)
		}
		var fluxo Centavos
		for _, o := range ops {
			if o.Data.Equal(data(dia)) {
				switch o.Tipo {
				case OpCompra:
					fluxo += Valor(o.Quantidade, o.Preco) + o.Taxas
				case OpVenda:
					fluxo -= Valor(o.Quantidade, o.Preco) - o.Taxas
				case OpProvento:
					fluxo -= o.Valor
				}
			}
		}
		pontos = append(pontos, PontoCarteira{Data: data(dia), Valor: Valor(p.Quantidade, d8(fechamento[dia])), Fluxo: fluxo})
	}
	twr, serie, err := TWR(pontos)
	if err != nil || math.Abs(twr*100-34.321900) > 0.01 || len(serie) != 7 {
		t.Fatalf("TWR %.6f %% (referência 34,321900 %%)", twr*100)
	}
	fluxos := []Fluxo{{data("2025-01-02"), -100500}, {data("2025-04-15"), -57750}, {data("2025-08-20"), 3000},
		{data("2025-10-10"), 74700}, {data("2025-12-31"), pontos[6].Valor}}
	x, err := XIRR(fluxos)
	if err != nil || math.Abs(x*100-29.559477) > 0.01 {
		t.Fatalf("XIRR %.6f %% (referência 29,559477 %%)", x*100)
	}

	// posição final: 90 ações, preço médio 10,55; venda com ganho de 114,00; provento 30,00
	p, _ := CalcularPosicao(ops, nil, data("2025-12-31"))
	if p.Quantidade != d8("90") || p.PrecoMedio != d8("10.55") || p.Custo != 94950 || p.Proventos != 3000 ||
		len(p.Vendas) != 1 || p.Vendas[0].Ganho != 11400 || p.Vendas[0].Custo != 63300 {
		t.Fatalf("posição: %+v", p)
	}
}

func TestEventosEDayTrade(t *testing.T) {
	ops := []Operacao{
		{Data: data("2026-01-05"), Tipo: OpCompra, Quantidade: d8("100"), Preco: d8("20")},
	}
	evs := []Evento{
		{Data: data("2026-02-01"), Tipo: EvDesdobramento, Fator: d8("2")},                         // 200 a 10,00
		{Data: data("2026-03-01"), Tipo: EvBonificacao, Fator: d8("1.1"), CustoUnitario: d8("5")}, // +20 a 5,00
		{Data: data("2026-04-01"), Tipo: EvGrupamento, Fator: d8("0.5")},                          // 110
	}
	p, err := CalcularPosicao(ops, evs, data("2026-04-30"))
	if err != nil || p.Quantidade != d8("110") || p.Custo != 210000 || p.PrecoMedio != PrecoMedio(210000, d8("110")) {
		t.Fatalf("eventos: %+v %v", p, err)
	}
	// ajuste como a B3 manda: +10 ações a 3,00 e depois −5 (sai sem mexer no custo)
	aj := append(evs, Evento{Data: data("2026-04-10"), Tipo: EvAjuste, Fator: d8("10"), CustoUnitario: d8("3")},
		Evento{Data: data("2026-04-11"), Tipo: EvAjuste, Fator: d8("-5")})
	if p, _ := CalcularPosicao(ops, aj, data("2026-04-30")); p.Quantidade != d8("115") || p.Custo != 213000 {
		t.Fatalf("ajuste: %+v", p)
	}
	// antes do desdobramento
	if p, _ := CalcularPosicao(ops, evs, data("2026-01-31")); p.Quantidade != d8("100") {
		t.Fatalf("data limite: %+v", p)
	}

	// day trade: compra 100 a 10 e vende 60 a 11 no mesmo dia; sobram 40 na posição
	dt := []Operacao{
		{Data: data("2026-05-04"), Tipo: OpCompra, Quantidade: d8("100"), Preco: d8("10"), Taxas: 100},
		{Data: data("2026-05-04"), Tipo: OpVenda, Quantidade: d8("60"), Preco: d8("11"), Taxas: 60, IRRetido: 6},
		{Data: data("2026-05-06"), Tipo: OpVenda, Quantidade: d8("40"), Preco: d8("9")},
		{Data: data("2026-05-07"), Tipo: OpJuros, Valor: 500, IRRetido: 75},
	}
	p, err = CalcularPosicao(dt, nil, data("2026-05-31"))
	if err != nil || len(p.Vendas) != 2 || p.Quantidade != 0 || p.Custo != 0 || p.Proventos != 425 {
		t.Fatalf("day trade: %+v %v", p, err)
	}
	v := p.Vendas[0]
	// custo 60/100 de (1000,00 + 1,00) = 600,60; venda 660,00 − 0,60 = 659,40
	if !v.DayTrade || v.Custo != 60060 || v.Valor != 65940 || v.Ganho != 5880 || v.IRRetido != 6 {
		t.Fatalf("venda day trade: %+v", v)
	}
	// comum: 40 ao custo 400,40, vendidas a 360,00
	if w := p.Vendas[1]; w.DayTrade || w.Custo != 40040 || w.Ganho != -4040 {
		t.Fatalf("venda comum: %+v", w)
	}

	// venda maior que a posição, e amortização abatendo o custo
	if _, err := CalcularPosicao([]Operacao{{Data: data("2026-01-01"), Tipo: OpVenda, Quantidade: d8("1"), Preco: d8("1")}}, nil, data("2026-02-01")); err != ErrVendaSemPosicao {
		t.Fatalf("venda a descoberto: %v", err)
	}
	am := []Operacao{
		{Data: data("2026-01-01"), Tipo: OpCompra, Quantidade: d8("1"), Preco: d8("1000")},
		{Data: data("2026-06-01"), Tipo: OpAmortizacao, Valor: 30000},
		{Data: data("2026-07-01"), Tipo: OpAmortizacao, Valor: 99999999},
	}
	if p, _ := CalcularPosicao(am, nil, data("2026-06-30")); p.Custo != 70000 {
		t.Fatalf("amortização: %+v", p)
	}
	if p, _ := CalcularPosicao(am, nil, data("2026-12-31")); p.Custo != 0 {
		t.Fatalf("amortização além do custo: %+v", p)
	}
}

var regras2026 = RegrasRendaVariavel{AliquotaComum: "0.15", AliquotaDayTrade: "0.20", AliquotaFII: "0.20",
	IsencaoAcoes: 2000000, DARFMinimo: 1000, CodigoDARF: "6015"}

func regrasFixas(time.Time) RegrasRendaVariavel { return regras2026 }

func venda(dia, classe string, valor, ganho Centavos, dayTrade bool, ir Centavos) VendaClassificada {
	return VendaClassificada{Venda: Venda{Data: data(dia), Valor: valor, Ganho: ganho, DayTrade: dayTrade, IRRetido: ir}, Classe: classe}
}

func TestApuracaoRendaVariavel(t *testing.T) {
	vs := []VendaClassificada{
		// jan: ações vendidas 15 mil com ganho 3 mil → isento; ETF com prejuízo de 500
		venda("2026-01-10", ClasseAcao, 1500000, 300000, false, 0),
		venda("2026-01-12", ClasseETF, 100000, -50000, false, 0),
		// fev: ações 30 mil (não isento) com ganho 2 mil; compensa 500 → base 1.500 → 225,00
		venda("2026-02-10", ClasseAcao, 3000000, 200000, false, 150),
		// fev: day trade com ganho 100 → 20,00; FII com ganho 40 → 8,00 (abaixo do mínimo junto com o resto? não: soma)
		venda("2026-02-11", ClasseAcao, 50000, 10000, true, 100),
		venda("2026-02-12", ClasseFII, 30000, 4000, false, 0),
		// mar: FII ganho pequeno → 2,00 → abaixo de 10,00, acumula
		venda("2026-03-10", ClasseFII, 10000, 1000, false, 0),
		// abr: ações 25 mil com prejuízo 1.000 (vai para o acumulado comum)
		venda("2026-04-10", ClasseAcao, 2500000, -100000, false, 0),
		// mai: ações isentas com prejuízo (continua compensável) e ganho em BDR 2.000 compensado
		venda("2026-05-10", ClasseAcao, 500000, -20000, false, 0),
		venda("2026-05-11", ClasseBDR, 400000, 200000, false, 0),
	}
	aps := ApurarRendaVariavel(vs, regrasFixas)
	if len(aps) != 5 {
		t.Fatalf("meses: %d", len(aps))
	}
	jan, fev, mar, abr, mai := aps[0], aps[1], aps[2], aps[3], aps[4]
	if !jan.Isento || jan.Imposto != 0 || jan.DARF != 0 || jan.Grupos[0].PrejuizoAcumulado != 50000 || jan.Grupos[0].Isento != 300000 {
		t.Fatalf("jan: %+v", jan)
	}
	if jan.Vencimento.Format("2006-01-02") != "2026-02-27" || jan.CodigoDARF != "6015" {
		t.Fatalf("vencimento de jan: %v", jan.Vencimento)
	}
	// fev: comum 225,00 + day trade 20,00 + FII 8,00 = 253,00 − IR retido 2,50 = 250,50
	if fev.Isento || fev.Imposto != 25300 || fev.IRRetido != 250 || fev.DARF != 25050 {
		t.Fatalf("fev: %+v", fev)
	}
	for _, g := range fev.Grupos {
		if g.Grupo == GrupoComum && (g.Base != 150000 || g.PrejuizoAnterior != 50000 || g.PrejuizoAcumulado != 0) {
			t.Fatalf("fev comum: %+v", g)
		}
	}
	if mar.DARF != 0 || mar.Acumulado != 200 {
		t.Fatalf("mar: %+v", mar)
	}
	// abr: prejuízo; DARF acumulado de março (2,00) continua abaixo do mínimo
	if abr.Imposto != 0 || abr.Acumulado != 200 || abr.Grupos[0].PrejuizoAcumulado != 100000 {
		t.Fatalf("abr: %+v", abr)
	}
	// mai: comum = −200 (ações, isentas) + 2.000 (BDR) = 1.800; compensa 1.000 → 800 × 15 % = 120,00 + 2,00 = 122,00
	if !mai.Isento || mai.Imposto != 12000 || mai.DARF != 12200 {
		t.Fatalf("mai: %+v", mai)
	}

	// IR retido maior que o imposto sobra para os meses seguintes
	sobra := ApurarRendaVariavel([]VendaClassificada{
		venda("2026-06-10", ClasseAcao, 3000000, 1000, false, 5000),
		venda("2026-07-10", ClasseAcao, 3000000, 100000, false, 0),
	}, regrasFixas)
	if sobra[0].DARF != 0 || sobra[1].DARF != 15000-(5000-150) {
		t.Fatalf("IR retido de sobra: %+v", sobra)
	}
	if aplicarAliquota(100, "x") != 0 || aplicarAliquota(-5, "0.15") != 0 {
		t.Fatal("alíquota inválida")
	}
}

func TestDiasUteis(t *testing.T) {
	// 2026: carnaval 16 e 17/02, sexta santa 03/04, Corpus Christi 04/06
	for _, f := range []string{"2026-02-16", "2026-02-17", "2026-04-03", "2026-06-04", "2026-11-20", "2026-12-25", "2026-01-01"} {
		if !Feriado(data(f)) || DiaUtil(data(f)) {
			t.Errorf("%s deveria ser feriado", f)
		}
	}
	if Feriado(data("2026-03-10")) || !DiaUtil(data("2026-03-10")) || DiaUtil(data("2026-03-14")) {
		t.Error("dia comum ou sábado")
	}
	// último dia útil: mai/2026 termina num domingo; out/2026 no sábado
	for mes, esperado := range map[string]string{"2026-05-01": "2026-05-29", "2026-10-01": "2026-10-30", "2026-12-01": "2026-12-31"} {
		if got := UltimoDiaUtil(data(mes)).Format("2006-01-02"); got != esperado {
			t.Errorf("último dia útil de %s: %s", mes, got)
		}
	}
	// janeiro de 2026 tem 21 dias úteis (1º é feriado)
	if n := DiasUteis(data("2025-12-31"), data("2026-01-31")); n != 21 {
		t.Errorf("dias úteis de jan/2026: %d", n)
	}
}

func TestRetornos(t *testing.T) {
	if _, _, err := TWR(nil); err != ErrSemDados {
		t.Error("TWR vazio")
	}
	// resgate total no meio: a base zerada não quebra a cota
	twr, _, _ := TWR([]PontoCarteira{
		{Data: data("2026-01-02"), Valor: 1000, Fluxo: 1000},
		{Data: data("2026-01-03"), Valor: 0, Fluxo: -1100},
		{Data: data("2026-01-04"), Valor: 0},
	})
	if twr != 0 {
		t.Errorf("TWR com resgate total: %v", twr)
	}
	if _, err := XIRR([]Fluxo{{data("2026-01-01"), -100}}); err != ErrSemDados {
		t.Error("XIRR sem entrada e saída")
	}
	// 10 % em exatamente um ano
	x, _ := XIRR([]Fluxo{{data("2025-01-01"), -10000}, {data("2026-01-01"), 11000}})
	if math.Abs(x-0.1) > 1e-6 {
		t.Errorf("XIRR de 10 %%: %v", x)
	}
	// perda quase total (Newton sai do domínio; a bisseção resolve)
	x, _ = XIRR([]Fluxo{{data("2026-01-01"), -1000000}, {data("2026-01-11"), 1}})
	if x > -0.99 {
		t.Errorf("XIRR de perda total: %v", x)
	}
	// Newton sai do domínio mas a raiz está no intervalo: a bisseção acha −99 % a.a.
	x, _ = XIRR([]Fluxo{{data("2025-01-01"), -10000}, {data("2026-01-01"), 100}})
	if math.Abs(x+0.99) > 1e-6 {
		t.Errorf("XIRR de −99 %%: %v", x)
	}
	// tudo na mesma data: sem derivada, fica com a ponta
	if _, err := XIRR([]Fluxo{{data("2026-01-01"), -100}, {data("2026-01-01"), 110}}); err != nil {
		t.Error(err)
	}
	// ganho absurdo em poucos dias: fora do intervalo pelo outro lado
	x, _ = XIRR([]Fluxo{{data("2026-01-01"), -1}, {data("2026-01-02"), 1000000}})
	if x < 99 {
		t.Errorf("XIRR de ganho extremo: %v", x)
	}
	if math.Abs(NoPeriodo(0.1, 365)-0.1) > 1e-12 || math.Abs(Acumular([]float64{0.1, 0.1})-0.21) > 1e-12 {
		t.Error("NoPeriodo/Acumular")
	}
}

func TestRendaFixaNaCurva(t *testing.T) {
	ap := data("2026-01-02")
	// 110 % do CDI por 3 dias úteis a 0,05 % ao dia
	cdi := []TaxaDia{{data("2026-01-02"), 0.0005}, {data("2026-01-05"), 0.0005}, {data("2026-01-06"), 0.0005}, {data("2026-01-07"), 0.0005}, {data("2026-01-08"), 0.0005}}
	v := ValorNaCurva(RendaFixa{Indexador: IdxCDI, Taxa: 1.10, Aplicacao: ap, Principal: 1000000}, data("2026-01-07"), cdi, nil)
	esperado := Centavos(math.Round(1000000 * math.Pow(1+0.0005*1.10, 3)))
	if v != esperado {
		t.Errorf("CDI: %d (esperado %d)", v, esperado)
	}
	// prefixado 12 % a.a. por 252 dias úteis = 12 %
	fim := ap
	for n := 0; n < 252; {
		fim = fim.AddDate(0, 0, 1)
		if DiaUtil(fim) {
			n++
		}
	}
	if v := ValorNaCurva(RendaFixa{Indexador: IdxPrefixado, Taxa: 0.12, Aplicacao: ap, Principal: 1000000}, fim, nil, nil); v != 1120000 {
		t.Errorf("prefixado: %d", v)
	}
	// IPCA + 6 %: um ano de IPCA 0,4 % ao mês (dezembro ainda não divulgado repete novembro)
	ipca := map[string]float64{}
	for m := 1; m <= 11; m++ {
		ipca[time.Date(2026, time.Month(m), 1, 0, 0, 0, 0, time.UTC).Format("2006-01")] = 0.004
	}
	v = ValorNaCurva(RendaFixa{Indexador: IdxIPCA, Taxa: 0.06, Aplicacao: data("2026-01-01"), Principal: 1000000}, data("2027-01-01"), nil, ipca)
	esperado = Centavos(math.Round(1000000 * math.Pow(1.004, 12) * math.Pow(1.06, float64(DiasUteis(data("2026-01-01"), data("2027-01-01")))/252)))
	if v != esperado {
		t.Errorf("IPCA+: %d (esperado %d)", v, esperado)
	}
	// aplicação no meio do mês: o primeiro mês conta pro rata
	v = ValorNaCurva(RendaFixa{Indexador: IdxIPCA, Taxa: 0, Aplicacao: data("2026-01-16"), Principal: 1000000}, data("2026-02-01"), nil, ipca)
	if esperado := Centavos(math.Round(1000000 * math.Pow(1.004, 16.0/31))); v != esperado {
		t.Errorf("IPCA pro rata: %d (esperado %d)", v, esperado)
	}
	// antes ou no dia da aplicação vale o principal
	if ValorNaCurva(RendaFixa{Indexador: IdxCDI, Aplicacao: ap, Principal: 500}, ap, cdi, nil) != 500 {
		t.Error("no dia da aplicação")
	}

	faixas := []FaixaIR{{180, "0.225"}, {360, "0.20"}, {720, "0.175"}, {0, "0.15"}}
	casos := map[int]Centavos{100: 2250, 200: 2000, 500: 1750, 1000: 1500}
	for dias, esperado := range casos {
		if got := IRRendaFixa(10000, dias, faixas, false); got != esperado {
			t.Errorf("IR com %d dias: %d", dias, got)
		}
	}
	if IRRendaFixa(10000, 10, faixas, true) != 0 || IRRendaFixa(-1, 10, faixas, false) != 0 || IRRendaFixa(10000, 10, nil, false) != 0 {
		t.Error("isento, prejuízo ou sem tabela")
	}
}
