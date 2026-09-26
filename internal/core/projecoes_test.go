package core

import (
	"math"
	"math/big"
	"testing"
	"time"
)

func dataUTC(a int, m time.Month, d int) time.Time { return time.Date(a, m, d, 0, 0, 0, 0, time.UTC) }

func TestDesvioPadrao(t *testing.T) {
	if DesvioPadrao(nil) != 0 || DesvioPadrao([]Centavos{5}) != 0 {
		t.Fatal("poucos valores")
	}
	if d := DesvioPadrao([]Centavos{2, 4, 4, 4, 5, 5, 7, 9}); d != 2 { // √(32/7) = 2,14
		t.Fatalf("desvio: %d", d)
	}
}

func TestVariavelDoMes(t *testing.T) {
	hoje := dataUTC(2026, 9, 15)
	h := map[string]Centavos{"2026-08": 3000, "2026-07": 2000, "2026-06": 1000, "2025-10": 4000, "2026-09": 999999}
	if v := VariavelDoMes(h, dataUTC(2026, 10, 1), hoje); v != 3000 { // (2000 + 4000) / 2
		t.Fatalf("outubro: %d", v)
	}
	if v := VariavelDoMes(h, dataUTC(2026, 11, 1), hoje); v != 2000 {
		t.Fatalf("novembro: %d", v)
	}
	if VariavelDoMes(map[string]Centavos{"2025-10": 1}, dataUTC(2026, 10, 1), hoje) != 0 {
		t.Fatal("sem meses recentes")
	}
}

func TestProjetarFluxo(t *testing.T) {
	f := ProjetarFluxo(EntradaFluxo{
		Hoje: dataUTC(2026, 9, 29), Dias: 2, SaldoInicial: 100000,
		Conhecidos: map[string]Centavos{"2026-09-30": -20000},
		Variaveis:  map[string]Centavos{"2026-08": 300000, "2026-07": 300000, "2026-06": 300000},
	})
	quer := []Centavos{100000, 70000, 60323} // 300000/30 = 10000 em setembro; 300000/31 em outubro
	if len(f) != 3 {
		t.Fatalf("dias: %d", len(f))
	}
	for i, q := range quer {
		if f[i].Base != q || f[i].Baixo != q || f[i].Alto != q {
			t.Fatalf("dia %d: %+v", i, f[i])
		}
	}
	// com variação nos meses a faixa abre com a raiz do tempo
	f = ProjetarFluxo(EntradaFluxo{Hoje: dataUTC(2026, 9, 1), Dias: 30, SaldoInicial: 0,
		Variaveis: map[string]Centavos{"2026-08": 1000, "2026-07": 3000}})
	if f[0].Alto != f[0].Base || f[30].Alto-f[30].Base != 1812 || f[30].Base-f[30].Baixo != 1812 { // 1,2816 × 1414
		t.Fatalf("faixa: %+v", f[30])
	}
}

func TestMonteCarloDeterministico(t *testing.T) {
	// sem volatilidade, todo cenário é igual: 6 % ao ano exatos
	p := MonteCarlo(EntradaMC{Classes: []ClasseMC{{Classe: "renda_fixa", Valor: 10_000_000,
		PremissaClasse: PremissaClasse{Retorno: 0.06}}}, Anos: 2, Cenarios: 10, Alvo: 10_500_000})
	if len(p) != 2 || p[0].P10 != 10_600_000 || p[0].P50 != 10_600_000 || p[0].P90 != 10_600_000 || p[1].P50 != 11_236_000 ||
		p[0].ChanceAlvo != 1 || p[0].Ano != 1 {
		t.Fatalf("determinístico: %+v", p)
	}
	// aporte dividido pelos pesos, sem rendimento
	p = MonteCarlo(EntradaMC{Classes: []ClasseMC{{Classe: "a", PesoAporte: 3}, {Classe: "b", PesoAporte: 1}},
		AporteMensal: 100_000, Anos: 1, Cenarios: 3, Alvo: 5_000_000})
	if p[0].P50 != 1_200_000 || p[0].ChanceAlvo != 0 {
		t.Fatalf("aportes: %+v", p)
	}
	// sem pesos, divide igual
	p = MonteCarlo(EntradaMC{Classes: []ClasseMC{{Classe: "a"}, {Classe: "b"}}, AporteMensal: 100_000, Anos: 1, Cenarios: 1})
	if p[0].P50 != 1_200_000 {
		t.Fatalf("sem pesos: %+v", p)
	}
	if MonteCarlo(EntradaMC{Anos: 1, Cenarios: 1}) != nil || MonteCarlo(EntradaMC{Classes: []ClasseMC{{}}, Cenarios: 1}) != nil {
		t.Fatal("entrada vazia")
	}
}

func TestMonteCarloAleatorio(t *testing.T) {
	e := EntradaMC{Classes: []ClasseMC{{Classe: "acao", Valor: 10_000_000, PremissaClasse: PremissasPadrao["acao"]}},
		Anos: 1, Cenarios: 5000, Semente: 42}
	p := MonteCarlo(e)
	// mediana da lognormal: V · (1 + r) · e^(−σ²/2)
	mediana := 10_000_000 * 1.07 * math.Exp(-0.25*0.25/2)
	if !(p[0].P10 < p[0].P50 && p[0].P50 < p[0].P90) || math.Abs(float64(p[0].P50)/mediana-1) > 0.02 {
		t.Fatalf("aleatório: %+v (mediana %.0f)", p, mediana)
	}
	if q := MonteCarlo(e); q[0] != p[0] {
		t.Fatal("mesma semente, mesmo resultado")
	}
}

func TestPercentil(t *testing.T) {
	if percentil([]float64{1, 2, 3, 4}, 0.5) != 2.5 || percentil([]float64{7}, 0.9) != 7 || percentil([]float64{1, 2}, 1) != 2 {
		t.Fatal("percentil")
	}
}

func TestIndependencia(t *testing.T) {
	if AlvoIndependencia(500_000, 0.04) != 150_000_000 || AlvoIndependencia(1, 0) != 0 {
		t.Fatal("alvo")
	}
	if MesesParaAlvo(10, 0, 5, 0) != 0 || MesesParaAlvo(0, 1000, 12000, 0) != 12 || MesesParaAlvo(0, 0, 1, 0) != -1 {
		t.Fatal("meses")
	}
	if m := MesesParaAlvo(0, 100_000, 10_000_000, 0.05); m <= 0 || m >= 100 {
		t.Fatalf("com rendimento chega antes de 100 meses: %d", m)
	}
}

func TestCompararFinanciamento(t *testing.T) {
	c, err := CompararFinanciamento(100_000, 10_000, 10, 1, 0.01)
	if err != nil || c.TotalParcelado != 100_000 || c.ValorPresenteParcelas != 94_713 || c.Diferenca != 5_287 ||
		!c.CompensaParcelar || math.Abs(c.JurosImplicitosMes) > 1e-6 {
		t.Fatalf("sem juros: %+v %v", c, err)
	}
	c, _ = CompararFinanciamento(90_000, 10_000, 10, 1, 0.01)
	if c.CompensaParcelar || math.Abs(c.JurosImplicitosMes-0.01963) > 1e-5 {
		t.Fatalf("com juros: %+v", c)
	}
	for _, e := range [][4]int{{0, 1, 1, 0}, {1, 0, 1, 0}, {1, 1, 0, 0}, {1, 1, 1, -1}, {1, 1, 1, 13}} {
		if _, err := CompararFinanciamento(Centavos(e[0]), Centavos(e[1]), e[2], e[3], 0.01); err == nil {
			t.Fatalf("inválido: %v", e)
		}
	}
	if _, err := CompararFinanciamento(1, 1, 1, 0, -1); err == nil {
		t.Fatal("rendimento inválido")
	}
}

func TestSimularAmortizacaoExtra(t *testing.T) {
	taxa := big.NewFloat(0.01)
	inicio := dataUTC(2026, 1, 10)
	price, _ := TabelaAmortizacao(SistemaPrice, 1_000_000, taxa, 12, inicio)
	data := dataUTC(2026, 3, 15) // três pagas (jan, fev, mar)

	s, err := SimularAmortizacaoExtra(SistemaPrice, price, taxa, data, 200_000, ReduzirPrazo)
	if err != nil || s.ParcelasRestantes != 9 || s.SaldoAntes != price[2].SaldoDevedor || s.SaldoDepois != s.SaldoAntes-200_000 ||
		s.NovasParcelas >= 9 || s.NovaPrestacao != s.PrestacaoAtual || s.Economia <= 0 || s.Economia != s.JurosRestantes-s.NovosJuros {
		t.Fatalf("prazo: %+v %v", s, err)
	}
	s, err = SimularAmortizacaoExtra(SistemaPrice, price, taxa, data, 200_000, ReduzirParcela)
	if err != nil || s.NovasParcelas != 9 || s.NovaPrestacao >= s.PrestacaoAtual || s.Economia <= 0 {
		t.Fatalf("parcela: %+v %v", s, err)
	}
	s, _ = SimularAmortizacaoExtra(SistemaPrice, price, taxa, data, 5_000_000, ReduzirPrazo)
	if !s.Quitada || s.Economia != s.JurosRestantes {
		t.Fatalf("quitada: %+v", s)
	}
	// antes da primeira parcela, o saldo é o principal
	s, _ = SimularAmortizacaoExtra(SistemaPrice, price, taxa, dataUTC(2026, 1, 1), 100_000, ReduzirPrazo)
	if s.SaldoAntes != 1_000_000 || s.ParcelasRestantes != 12 {
		t.Fatalf("antes de começar: %+v", s)
	}
	sac, _ := TabelaAmortizacao(SistemaSAC, 1_200_000, taxa, 12, inicio)
	s, err = SimularAmortizacaoExtra(SistemaSAC, sac, taxa, data, 250_000, ReduzirPrazo)
	if err != nil || s.NovasParcelas != 7 || s.Economia <= 0 { // 650.000 com 100.000 de amortização: 7 parcelas
		t.Fatalf("sac: %+v %v", s, err)
	}
	for _, c := range []struct {
		tab   []ParcelaDivida
		extra Centavos
		modo  string
		data  time.Time
	}{{nil, 1, ReduzirPrazo, data}, {price, 0, ReduzirPrazo, data}, {price, 1, "x", data}, {price, 1, ReduzirPrazo, dataUTC(2030, 1, 1)}} {
		if _, err := SimularAmortizacaoExtra(SistemaPrice, c.tab, taxa, c.data, c.extra, c.modo); err == nil {
			t.Fatalf("inválido: %+v", c)
		}
	}
	if _, err := SimularAmortizacaoExtra("x", price, taxa, data, 1, ReduzirParcela); err == nil {
		t.Fatal("sistema inválido")
	}
}
