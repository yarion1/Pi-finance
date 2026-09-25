package core

import (
	"reflect"
	"testing"
)

func TestFaturaDaCompra(t *testing.T) {
	casos := []struct {
		compra         string
		fech, venc     int
		quer, querVenc string
	}{
		{"2026-09-02", 5, 12, "2026-09-05", "2026-09-12"},  // antes do fechamento: fatura do mês
		{"2026-09-05", 5, 12, "2026-10-05", "2026-10-12"},  // no dia do fechamento: próxima
		{"2026-09-20", 5, 12, "2026-10-05", "2026-10-12"},  // depois: próxima
		{"2026-09-20", 25, 3, "2026-09-25", "2026-10-03"},  // vencimento no mês seguinte ao fechamento
		{"2026-12-28", 25, 3, "2027-01-25", "2027-02-03"},  // virada de ano
		{"2026-01-30", 31, 8, "2026-01-31", "2026-02-08"},  // fechamento 31 em janeiro
		{"2026-02-10", 31, 8, "2026-02-28", "2026-03-08"},  // 31 vira 28 em fevereiro
		{"2028-02-29", 29, 10, "2028-03-29", "2028-04-10"}, // bissexto, no dia
	}
	for _, c := range casos {
		f, v := FaturaDaCompra(dia(c.compra), c.fech, c.venc)
		if f.Format("2006-01-02") != c.quer || v.Format("2006-01-02") != c.querVenc {
			t.Errorf("compra %s (fecha %d, vence %d): %s/%s, esperado %s/%s", c.compra, c.fech, c.venc,
				f.Format("2006-01-02"), v.Format("2006-01-02"), c.quer, c.querVenc)
		}
		if got := FechamentoDaFatura(v, c.fech, c.venc); !got.Equal(f) {
			t.Errorf("FechamentoDaFatura(%s) = %s, esperado %s", v.Format("2006-01-02"), got.Format("2006-01-02"), f.Format("2006-01-02"))
		}
	}
}

// Critério de aceite da fase 2: R$ 1.200 em 12× cai nas 12 faturas seguintes, uma por mês.
func TestParcelasNasFaturasCertas(t *testing.T) {
	parcelas := DividirParcelas(-120000, 12)
	compra := dia("2026-09-20") // depois do fechamento (dia 5): 1ª parcela na fatura de outubro
	vistos := map[string]bool{}
	var soma Centavos
	for i, p := range parcelas {
		soma += p
		_, venc := FaturaDaCompra(SomarMeses(compra, i), 5, 12)
		chave := venc.Format("2006-01")
		if vistos[chave] {
			t.Fatalf("duas parcelas na fatura %s", chave)
		}
		vistos[chave] = true
		if i == 0 && chave != "2026-10" || i == 11 && chave != "2027-09" {
			t.Errorf("parcela %d na fatura %s", i+1, chave)
		}
	}
	if soma != -120000 || len(vistos) != 12 {
		t.Fatalf("soma %d em %d faturas", soma, len(vistos))
	}
}

func TestDividirParcelas(t *testing.T) {
	if got := DividirParcelas(100000, 3); !reflect.DeepEqual(got, []Centavos{33334, 33333, 33333}) {
		t.Errorf("1.000 em 3: %v", got)
	}
	if got := DividirParcelas(-1000, 4); !reflect.DeepEqual(got, []Centavos{-250, -250, -250, -250}) {
		t.Errorf("-10 em 4: %v", got)
	}
	if DividirParcelas(1, 0) != nil {
		t.Error("0 parcelas")
	}
}

func TestSomarMeses(t *testing.T) {
	if got := SomarMeses(dia("2026-01-31"), 1); got.Format("2006-01-02") != "2026-02-28" {
		t.Error(got)
	}
	if got := SomarMeses(dia("2026-11-15"), 3); got.Format("2006-01-02") != "2027-02-15" {
		t.Error(got)
	}
}

func TestExtrairParcela(t *testing.T) {
	casos := []struct {
		entrada, base string
		n, total      int
		ok            bool
	}{
		{"Magazine Luiza - Parcela 3/12", "Magazine Luiza", 3, 12, true},
		{"LOJA X PARC 03/10", "LOJA X", 3, 10, true},
		{"Geladeira (1/12)", "Geladeira", 1, 12, true},
		{"Parcela 2 de 6 - Curso", "Curso", 2, 6, true},
		{"Padaria 12/09", "Padaria 12/09", 0, 0, false}, // data, não parcela
		{"Parcela 13/12", "Parcela 13/12", 0, 0, false},
		{"Mercado", "Mercado", 0, 0, false},
	}
	for _, c := range casos {
		base, n, total, ok := ExtrairParcela(c.entrada)
		if base != c.base || n != c.n || total != c.total || ok != c.ok {
			t.Errorf("%q → %q %d/%d %v", c.entrada, base, n, total, ok)
		}
	}
}
