package core

import (
	"testing"
	"time"
)

// Tabelas vigentes em 2026 (as mesmas das migrações 00002 e 00012).
var (
	faixasIII = []FaixaSimples{
		{18000000, "0.06", 0}, {36000000, "0.112", 936000}, {72000000, "0.135", 1764000},
		{180000000, "0.16", 3564000}, {360000000, "0.21", 12564000}, {480000000, "0.33", 64800000},
	}
	faixasV = []FaixaSimples{
		{18000000, "0.155", 0}, {36000000, "0.18", 450000}, {72000000, "0.195", 990000},
		{180000000, "0.205", 1710000}, {360000000, "0.23", 6210000}, {480000000, "0.305", 54000000},
	}
	partilhaIII = []map[string]string{
		{"irpj": "0.04", "csll": "0.035", "cofins": "0.1282", "pis": "0.0278", "cpp": "0.434", "iss": "0.335"},
		{"irpj": "0.04", "csll": "0.035", "cofins": "0.1405", "pis": "0.0305", "cpp": "0.434", "iss": "0.32"},
		{"irpj": "0.04", "csll": "0.035", "cofins": "0.1364", "pis": "0.0296", "cpp": "0.434", "iss": "0.325"},
		{"irpj": "0.04", "csll": "0.035", "cofins": "0.1364", "pis": "0.0296", "cpp": "0.434", "iss": "0.325"},
		{"irpj": "0.04", "csll": "0.035", "cofins": "0.1282", "pis": "0.0278", "cpp": "0.434", "iss": "0.335"},
		{"irpj": "0.35", "csll": "0.15", "cofins": "0.1603", "pis": "0.0347", "cpp": "0.305", "iss": "0"},
	}
	partilhaV = []map[string]string{
		{"irpj": "0.25", "csll": "0.15", "cofins": "0.141", "pis": "0.0305", "cpp": "0.2885", "iss": "0.14"},
		{"irpj": "0.23", "csll": "0.15", "cofins": "0.141", "pis": "0.0305", "cpp": "0.2785", "iss": "0.17"},
		{"irpj": "0.24", "csll": "0.15", "cofins": "0.1492", "pis": "0.0323", "cpp": "0.2385", "iss": "0.19"},
		{"irpj": "0.21", "csll": "0.15", "cofins": "0.1574", "pis": "0.0341", "cpp": "0.2385", "iss": "0.21"},
		{"irpj": "0.23", "csll": "0.125", "cofins": "0.141", "pis": "0.0305", "cpp": "0.2385", "iss": "0.235"},
		{"irpj": "0.35", "csll": "0.155", "cofins": "0.1644", "pis": "0.0356", "cpp": "0.295", "iss": "0"},
	}
)

func das(t *testing.T, anexo string, rbt12, receita, exportacao Centavos) ResultadoDAS {
	t.Helper()
	f, p := faixasIII, partilhaIII
	if anexo == AnexoV {
		f, p = faixasV, partilhaV
	}
	r, err := CalcularDAS(ParametrosDAS{Anexo: anexo, Faixas: f, Partilha: p, ISSMaximo: "0.05",
		RBT12: rbt12, Receita: receita, Exportacao: exportacao})
	if err != nil {
		t.Fatal(err)
	}
	var soma Centavos
	for _, x := range r.Tributos {
		soma += x.Valor
	}
	if soma != r.DAS {
		t.Fatalf("partilha soma %d, DAS %d", soma, r.DAS)
	}
	return r
}

// Critério de aceite da fase 5: 12 casos fixos, uma receita por faixa em cada anexo, com
// o DAS calculado à mão: receita × (RBT12 × nominal − deduzir) ÷ RBT12, ao centavo.
func TestDASDozeCasos(t *testing.T) {
	casos := []struct {
		anexo          string
		rbt12, receita Centavos
		faixa          int
		efetiva        string
		das            Centavos
	}{
		// Anexo III
		{AnexoIII, 12000000, 1000000, 1, "0.06000000", 60000},     // 10.000 × 6 %
		{AnexoIII, 24000000, 2000000, 2, "0.07300000", 146000},    // (26.880 − 9.360)/240.000
		{AnexoIII, 50000000, 4000000, 3, "0.09972000", 398880},    // (67.500 − 17.640)/500.000
		{AnexoIII, 100000000, 8333333, 4, "0.12436000", 1036333},  // 83.333,33 × 12,436 % = 10.363,33
		{AnexoIII, 250000000, 20000000, 5, "0.15974400", 3194880}, // (525.000 − 125.640)/2.500.000
		{AnexoIII, 400000000, 35000000, 6, "0.16800000", 5880000}, // (1.320.000 − 648.000)/4.000.000
		// Anexo V
		{AnexoV, 15000000, 1250000, 1, "0.15500000", 193750},    // 12.500 × 15,5 %
		{AnexoV, 24000000, 2000000, 2, "0.16125000", 322500},    // exemplo da SPEC: 16,13 % e R$ 3.225
		{AnexoV, 60000000, 5000000, 3, "0.17850000", 892500},    // (117.000 − 9.900)/600.000
		{AnexoV, 120000000, 10000001, 4, "0.19075000", 1907500}, // 100.000,01 × 19,075 % = 19.075,0019
		{AnexoV, 300000000, 25000000, 5, "0.20930000", 5232500}, // (690.000 − 62.100)/3.000.000
		{AnexoV, 450000000, 37500000, 6, "0.18500000", 6937500}, // (1.372.500 − 540.000)/4.500.000
	}
	for _, c := range casos {
		r := das(t, c.anexo, c.rbt12, c.receita, 0)
		if r.Faixa != c.faixa || r.AliquotaEfetiva != c.efetiva || r.DAS != c.das {
			t.Errorf("Anexo %s faixa %d: efetiva %s DAS %d, quer %s %d", c.anexo, c.faixa, r.AliquotaEfetiva, r.DAS, c.efetiva, c.das)
		}
	}
}

// Exemplo da SPEC §4: RBT12 de R$ 240 mil, R$ 20 mil no mês; Anexo V 16,13 % (R$ 3.225)
// contra Anexo III 7,30 % (R$ 1.460) com pró-labore de R$ 5.600 (INSS de R$ 616).
func TestExemploDaSPEC(t *testing.T) {
	v := das(t, AnexoV, 24000000, 2000000, 0)
	iii := das(t, AnexoIII, 24000000, 2000000, 0)
	if v.DAS != 322500 || iii.DAS != 146000 || iii.AliquotaEfetiva != "0.07300000" {
		t.Fatalf("V %d, III %d %s", v.DAS, iii.DAS, iii.AliquotaEfetiva)
	}
	pl := ProlaboreParaFatorR(24000000, 0, 162100, "0.28")
	if pl != 560000 {
		t.Fatalf("pró-labore mínimo para 28 %%: %d", pl)
	}
	if FatorR(pl*12, 24000000) != "0.28000000" || AnexoPeloFatorR(pl*12, 24000000, "0.28") != AnexoIII {
		t.Fatal("Fator R de 28 % leva ao Anexo III")
	}
	if AnexoPeloFatorR(pl*12-1, 24000000, "0.28") != AnexoV {
		t.Fatal("um centavo abaixo fica no V")
	}
	if inss := INSSProlabore(pl, RegrasINSS{Aliquota: "0.11", Teto: 847555}); inss != 61600 {
		t.Fatalf("INSS: %d", inss)
	}
}

func TestPartilhaETetoDoISS(t *testing.T) {
	// faixa 1 do Anexo III: ISS = 6 % × 33,5 % = 2,01 %
	r := das(t, AnexoIII, 12000000, 1000000, 0)
	if r.Tributos[5].Tributo != "iss" || r.Tributos[5].Valor != 20100 || r.Tributos[4].Valor != 26040 {
		t.Fatalf("partilha: %+v", r.Tributos)
	}
	// faixa 5 do Anexo III com RBT12 de R$ 3,5 mi: efetiva 17,41 %; ISS seria 5,83 %, fica em 5 %
	r = das(t, AnexoIII, 350000000, 10000000, 0)
	if r.Tributos[5].Aliquota != "0.05000000" || r.Tributos[5].Valor != 500000 {
		t.Fatalf("teto do ISS: %+v", r.Tributos[5])
	}
	// a sobra vai para os federais na proporção: CPP = 43,4 % ÷ 66,5 % de (17,41 − 5) %
	if r.Tributos[4].Aliquota != "0.08099344" {
		t.Fatalf("CPP com a sobra do ISS: %+v", r.Tributos[4])
	}
	if r.AliquotaEfetiva != "0.17410286" || r.DAS != 1741029 {
		t.Fatalf("efetiva %s DAS %d", r.AliquotaEfetiva, r.DAS)
	}
}

func TestExportacao(t *testing.T) {
	// R$ 20 mil, metade exportação, Anexo III faixa 2 (7,3 %): sem PIS, COFINS e ISS na metade
	r := das(t, AnexoIII, 24000000, 2000000, 1000000)
	// PIS + COFINS + ISS = 3,05 + 14,05 + 32 = 49,1 % da alíquota; a metade exportada paga 50,9 %
	// DAS = 10.000 × 7,3 % + 10.000 × 7,3 % × 50,9 % = 730 + 371,57 = 1.101,57
	if r.DAS != 110157 || r.SemExportacao != 146000 {
		t.Fatalf("exportação: DAS %d sem exportação %d", r.DAS, r.SemExportacao)
	}
	tudo := das(t, AnexoV, 24000000, 2000000, 2000000)
	for _, x := range tudo.Tributos {
		if tributosExportacao[x.Tributo] && x.Valor != 0 {
			t.Fatalf("%s na exportação: %d", x.Tributo, x.Valor)
		}
	}
}

func TestDASErros(t *testing.T) {
	base := ParametrosDAS{Anexo: AnexoIII, Faixas: faixasIII, Partilha: partilhaIII, RBT12: 100, Receita: 100}
	casos := map[string]func(p *ParametrosDAS){
		"acima do teto":        func(p *ParametrosDAS) { p.RBT12 = 480000001 },
		"exportação > receita": func(p *ParametrosDAS) { p.Exportacao = 101 },
		"sem faixas":           func(p *ParametrosDAS) { p.Faixas = nil },
		"sem partilha":         func(p *ParametrosDAS) { p.Partilha = nil },
		"alíquota ruim":        func(p *ParametrosDAS) { p.Faixas = []FaixaSimples{{Ate: 1000, Aliquota: "x"}} },
		"partilha ruim": func(p *ParametrosDAS) {
			p.Partilha = []map[string]string{{"irpj": "x"}}
		},
		"partilha não soma 1": func(p *ParametrosDAS) {
			p.Partilha = []map[string]string{{"irpj": "0.5"}}
		},
	}
	for nome, mudar := range casos {
		p := base
		mudar(&p)
		if _, err := CalcularDAS(p); err == nil {
			t.Errorf("%s: esperava erro", nome)
		}
	}
	// sem RBT12 (primeiro mês): a nominal da primeira faixa; sem teto de ISS informado
	p := base
	p.RBT12, p.Receita = 0, 500000
	r, err := CalcularDAS(p)
	if err != nil || r.AliquotaEfetiva != "0.06000000" || r.DAS != 30000 {
		t.Fatalf("primeiro mês: %+v %v", r, err)
	}
}

func TestRBT12(t *testing.T) {
	doze := []Centavos{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 1000}
	if RBT12(doze, 0) != 1000+2+3+4+5+6+7+8+9+10+11+12 {
		t.Fatal("últimos 12")
	}
	if RBT12(nil, 500000) != 6000000 {
		t.Fatal("primeiro mês: receita × 12")
	}
	if RBT12([]Centavos{1000000, 2000000, 3000000}, 0) != 24000000 {
		t.Fatal("média × 12")
	}
	if FatorR(100, 0) != "0.00000000" || AnexoPeloFatorR(0, 0, "0.28") != AnexoV ||
		AnexoPeloFatorR(1, 0, "0.28") != AnexoIII || AnexoPeloFatorR(1, 1, "x") != AnexoV {
		t.Fatal("Fator R nas bordas")
	}
	if ProlaboreParaFatorR(0, 0, 162100, "0.28") != 162100 ||
		ProlaboreParaFatorR(10000000, 5000000, 162100, "0.28") != 162100 ||
		ProlaboreParaFatorR(10000001, 0, 100, "0.28") != 233334 {
		t.Fatal("pró-labore nas bordas")
	}
}

func TestMEI(t *testing.T) {
	abertura := time.Date(2026, 4, 10, 0, 0, 0, 0, time.UTC)
	if TetoMEI(&abertura, 2026, 8100000, 675000) != 6075000 || TetoMEI(&abertura, 2027, 8100000, 675000) != 8100000 ||
		TetoMEI(nil, 2026, 8100000, 675000) != 8100000 {
		t.Fatal("teto proporcional: abril a dezembro = 9 × 6.750")
	}
	for fat, quer := range map[Centavos]string{
		5000000: MEINormal, 5670000: MEIAtencao, 7290000: MEIPlanejar, 8100000: MEIPlanejar,
		8100001: MEIViraME, 9720000: MEIViraME, 9720001: MEIDesenquadrado,
	} {
		if s := SituacaoMEI(fat, 8100000, "0.20"); s != quer {
			t.Errorf("%d: %s, quer %s", fat, s, quer)
		}
	}
	if SituacaoMEI(1, 0, "0.20") != MEINormal || SituacaoMEI(8100001, 8100000, "x") != MEIDesenquadrado {
		t.Fatal("bordas da situação")
	}

	// R$ 54 mil até 30/06 (181 dias): no ritmo, bate R$ 81 mil no dia 272 (29/09)
	hoje := time.Date(2026, 6, 30, 15, 0, 0, 0, time.UTC)
	p := ProjetarMEI(5400000, 8100000, hoje, nil)
	if p.Falta != 2700000 || p.MesesRestantes != 7 || p.MediaMaxima != 385714 || p.DataTeto == nil ||
		p.DataTeto.Format("2006-01-02") != "2026-09-29" || p.ProjecaoAno != 10889503 {
		t.Fatalf("projeção: %+v %v", p, p.DataTeto)
	}
	// ritmo baixo: não bate no ano
	if p := ProjetarMEI(100000, 8100000, hoje, nil); p.DataTeto != nil {
		t.Fatal("não bate o teto no ano")
	}
	// já passou: a data é hoje
	if p := ProjetarMEI(8200000, 8100000, hoje, nil); p.DataTeto == nil || p.Falta != 0 {
		t.Fatal("teto já batido")
	}
	// sem faturamento; e aberto no ano, o ritmo conta desde a abertura
	if p := ProjetarMEI(0, 8100000, hoje, nil); p.ProjecaoAno != 0 || p.DataTeto != nil {
		t.Fatal("sem faturamento")
	}
	if p := ProjetarMEI(1000000, 6075000, hoje, &abertura); !p.Inicio.Equal(abertura) || p.ProjecaoAno != 3243902 {
		t.Fatalf("desde a abertura: %+v", p)
	}
	if p := ProjetarMEI(1, 0, hoje, nil); p.Percentual != 0 {
		t.Fatal("sem teto")
	}

	r := RegrasDASMEI{INSS: 8105, ISS: 500, ICMS: 100, VencimentoDia: 20}
	if DASMEI(r, "servicos") != 8605 || DASMEI(r, "comercio") != 8205 || DASMEI(r, "ambos") != 8705 || DASMEI(r, "") != 8605 {
		t.Fatal("DAS-MEI 2026: 86,05 / 82,05 / 87,05")
	}

	l := CalcularLucroMEI(6000000, 1000000, 1500000, "0.32", "0.08")
	if l.Lucro != 5500000 || l.Presumido != 2000000 || l.Isento != 2000000 || l.Tributavel != 3500000 {
		t.Fatalf("lucro: %+v", l)
	}
	// lucro menor que o presumido: tudo isento; prejuízo: nada
	if l := CalcularLucroMEI(6000000, 0, 5000000, "0.32", "x"); l.Isento != 1000000 || l.Tributavel != 0 {
		t.Fatalf("lucro pequeno: %+v", l)
	}
	if l := CalcularLucroMEI(100, 0, 500, "0.32", "0.08"); l.Isento != 0 || l.Tributavel != 0 {
		t.Fatalf("prejuízo: %+v", l)
	}
}

var tabela2026 = TabelaIRRF{
	Faixas: []FaixaIRRF{
		{242880, "0", 0}, {282665, "0.075", 18216}, {375105, "0.15", 39416}, {466468, "0.225", 67549}, {0, "0.275", 90873},
	},
	DescontoSimplificado: 60720,
	Dependente:           18959,
	Reducao:              &ReducaoIRRF{Ate: 500000, Maxima: 31289, AteParcial: 735000, Fixo: 97862, Coeficiente: "0.133145"},
}

func TestIRRF2026(t *testing.T) {
	inss := func(v Centavos) Centavos { return INSSProlabore(v, RegrasINSS{Aliquota: "0.11", Teto: 847555}) }
	casos := []struct {
		rend Centavos
		dep  int
		quer Centavos
	}{
		{0, 0, 0},
		{50000, 0, 0},  // abaixo do desconto simplificado
		{162100, 0, 0}, // salário mínimo
		{500000, 0, 0}, // até R$ 5 mil: zerado (4.392,80 × 22,5 % − 675,49 = 312,89 − 312,89)
		// R$ 5.600: INSS 616 > simplificado; base 4.984 × 27,5 % − 908,73 = 461,87;
		// redução 978,62 − 0,133145 × 5.600 = 233,01 → 228,86
		{560000, 0, 22886},
		// R$ 10 mil: INSS no teto (932,31); base 9.067,69 × 27,5 % − 908,73 = 1.584,88; sem redução
		{1000000, 0, 158488},
		// R$ 3 mil com 2 dependentes: 330 + 379,18 = 709,18 > 607,20; base 2.290,82 → isento
		{300000, 2, 0},
	}
	for _, c := range casos {
		if v := IRRF(c.rend, inss(c.rend), c.dep, tabela2026); v != c.quer {
			t.Errorf("IRRF de %d: %d, quer %d", c.rend, v, c.quer)
		}
	}
	// sem redução (tabela antiga) e tabela ruim
	antiga := tabela2026
	antiga.Reducao = nil
	if v := IRRF(500000, 55000, 0, antiga); v != 31289 {
		t.Fatalf("sem a redução: %d", v)
	}
	ruim := TabelaIRRF{Faixas: []FaixaIRRF{{0, "x", 0}}}
	if IRRF(500000, 0, 0, ruim) != 0 || INSSProlabore(100, RegrasINSS{Aliquota: "x"}) != 0 ||
		INSSProlabore(100000, RegrasINSS{Aliquota: "0.11"}) != 11000 {
		t.Fatal("bordas")
	}
	coef := tabela2026
	coef.Reducao = &ReducaoIRRF{Ate: 500000, Maxima: 31289, AteParcial: 735000, Fixo: 97862, Coeficiente: "x"}
	if v := IRRF(560000, 61600, 0, coef); v != 46187 {
		t.Fatalf("coeficiente inválido não reduz: %d", v)
	}
}

func TestDividendos(t *testing.T) {
	l := LimiteDividendos{Centavos: 5000000, AliquotaIRRF: "0.10"}
	if irrf, folga := IRRFDividendos(3000000, 1000000, l); irrf != 0 || folga != 1000000 {
		t.Fatalf("abaixo: %d %d", irrf, folga)
	}
	if irrf, folga := IRRFDividendos(3000000, 3000000, l); irrf != 600000 || folga != 0 {
		t.Fatalf("acima: 10 %% sobre o total de R$ 60 mil: %d %d", irrf, folga)
	}
	if irrf, _ := IRRFDividendos(0, 6000000, LimiteDividendos{Centavos: 5000000, AliquotaIRRF: "x"}); irrf != 0 {
		t.Fatal("alíquota inválida")
	}
}
