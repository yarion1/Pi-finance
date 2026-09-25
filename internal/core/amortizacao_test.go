package core

import "testing"

func TestTabelaPrice(t *testing.T) {
	taxa, _ := LerTaxa("0.01")
	tab, err := TabelaAmortizacao(SistemaPrice, 10000000, taxa, 12, dia("2026-10-10"))
	if err != nil {
		t.Fatal(err)
	}
	// R$ 100.000 a 1 % a.m. em 12×: prestação R$ 8.884,88 (calculadora do cidadão / HP-12C)
	p1 := tab[0]
	if p1.Prestacao != 888488 || p1.Juros != 100000 || p1.Amortizacao != 788488 || p1.SaldoDevedor != 9211512 {
		t.Fatalf("1ª parcela: %+v", p1)
	}
	var amortizado Centavos
	for _, p := range tab[:11] {
		if p.Prestacao != 888488 {
			t.Errorf("parcela %d com prestação %d", p.Numero, p.Prestacao)
		}
	}
	for _, p := range tab {
		amortizado += p.Amortizacao
	}
	ultima := tab[11]
	if ultima.SaldoDevedor != 0 || amortizado != 10000000 || ultima.Vencimento.Format("2006-01-02") != "2027-09-10" {
		t.Fatalf("última: %+v, amortizado %d", ultima, amortizado)
	}
	if d := ultima.Prestacao - 888488; d < -12 || d > 12 {
		t.Errorf("a última só ajusta centavos: %d", ultima.Prestacao)
	}
	if s := SaldoDevedorEm(tab, 10000000, dia("2026-10-09")); s != 10000000 {
		t.Errorf("antes da 1ª parcela: %d", s)
	}
	if s := SaldoDevedorEm(tab, 10000000, dia("2026-11-10")); s != tab[1].SaldoDevedor {
		t.Errorf("depois da 2ª: %d", s)
	}
}

func TestTabelaSAC(t *testing.T) {
	taxa, _ := LerTaxa("0,01")
	tab, err := TabelaAmortizacao(SistemaSAC, 12000000, taxa, 12, dia("2026-10-10"))
	if err != nil {
		t.Fatal(err)
	}
	if tab[0].Amortizacao != 1000000 || tab[0].Juros != 120000 || tab[0].Prestacao != 1120000 {
		t.Fatalf("1ª SAC: %+v", tab[0])
	}
	if tab[11].Juros != 10000 || tab[11].Prestacao != 1010000 || tab[11].SaldoDevedor != 0 {
		t.Fatalf("última SAC: %+v", tab[11])
	}
	for i := 1; i < 12; i++ {
		if tab[i].Prestacao >= tab[i-1].Prestacao {
			t.Fatalf("no SAC a prestação cai: %d ≥ %d", tab[i].Prestacao, tab[i-1].Prestacao)
		}
	}
}

func TestAmortizacaoSemJurosEErros(t *testing.T) {
	zero, _ := LerTaxa("0")
	tab, _ := TabelaAmortizacao(SistemaPrice, 100000, zero, 3, dia("2026-01-31"))
	if tab[0].Prestacao != 33333 || tab[2].Prestacao != 33334 || tab[2].SaldoDevedor != 0 || tab[1].Vencimento.Format("2006-01-02") != "2026-02-28" {
		t.Fatalf("sem juros: %+v", tab)
	}
	if _, err := LerTaxa("abc"); err == nil {
		t.Error("taxa inválida")
	}
	if _, err := LerTaxa("1.5"); err == nil {
		t.Error("taxa de 150 % ao mês não é razoável")
	}
	if _, err := TabelaAmortizacao("outro", 1, zero, 1, dia("2026-01-01")); err == nil {
		t.Error("sistema desconhecido")
	}
	if _, err := TabelaAmortizacao(SistemaSAC, 0, zero, 1, dia("2026-01-01")); err == nil {
		t.Error("principal zero")
	}
}
