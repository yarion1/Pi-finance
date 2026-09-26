package core

import (
	"strings"
	"testing"
	"time"
)

func TestMesDoRelatorio(t *testing.T) {
	if m := MesDoRelatorio(dataUTC(2026, 1, 1)); !m.Equal(dataUTC(2025, 12, 1)) {
		t.Fatalf("janeiro: %v", m)
	}
	if m := MesDoRelatorio(dataUTC(2026, 9, 26)); !m.Equal(dataUTC(2026, 8, 1)) {
		t.Fatalf("setembro: %v", m)
	}
}

func TestSemanaDoResumo(t *testing.T) {
	h := func(d, hora int) time.Time { return time.Date(2026, 9, d, hora, 0, 0, 0, time.UTC) }
	casos := map[time.Time]time.Time{
		h(27, 18): dataUTC(2026, 9, 14), // domingo antes das 19 h: a semana anterior
		h(27, 19): dataUTC(2026, 9, 21), // domingo 19 h: a própria semana
		h(28, 8):  dataUTC(2026, 9, 21), // segunda: a que acabou ontem
		h(26, 23): dataUTC(2026, 9, 14), // sábado
	}
	for agora, quer := range casos {
		if s := SemanaDoResumo(agora); !s.Equal(quer) {
			t.Errorf("%v: %v, quer %v", agora, s, quer)
		}
	}
}

func TestSugerirAcoes(t *testing.T) {
	tem := func(a []string, i int, trecho string) bool {
		return i < len(a) && strings.Contains(strings.ReplaceAll(a[i], "\u00a0", " "), trecho)
	}
	a := SugerirAcoes(EntradaAcoes{Receitas: 500_000, Gastos: 600_000, CategoriaQueSubiu: "Lazer", AltaCategoria: 30_000,
		AltaPercentual: 0.5, Estouradas: []string{"Mercado"}, Reserva: 100_000, CustoMedio: 600_000})
	if len(a) != 3 || !tem(a, 0, "R$ 1.000,00") || !tem(a, 1, "Lazer subiu R$ 300,00 (50 %)") || !tem(a, 2, "Mercado passou do orçamento") {
		t.Fatalf("urgentes: %q", a)
	}
	a = SugerirAcoes(EntradaAcoes{Receitas: 800_000, Gastos: 600_000, CategoriaQueSubiu: "Lazer", AltaCategoria: 5_000,
		AltaPercentual: 0.5, Estouradas: []string{"Mercado", "Lazer"}, Reserva: 900_000, CustoMedio: 600_000})
	if len(a) != 3 || !tem(a, 0, "2 categorias passaram do orçamento, a começar por Mercado") ||
		!tem(a, 1, "cobre 1,5 mês") || !tem(a, 2, "Sobraram R$ 2.000,00") {
		t.Fatalf("rotina: %q", a)
	}
	a = SugerirAcoes(EntradaAcoes{Reserva: 1_200_000, CustoMedio: 600_000, Assinaturas: 9_990, SemCategoria: 4})
	if len(a) != 3 || !tem(a, 0, "cobre 2,0 meses") || !tem(a, 1, "assinaturas somam R$ 99,90") || !tem(a, 2, "4 transações") {
		t.Fatalf("assinaturas: %q", a)
	}
	a = SugerirAcoes(EntradaAcoes{Reserva: 6_000_000, CustoMedio: 600_000, SemCategoria: 1})
	if len(a) != 1 || !tem(a, 0, "1 transação ficou") {
		t.Fatalf("reserva boa: %q", a)
	}
	if len(SugerirAcoes(EntradaAcoes{})) != 0 {
		t.Fatal("nada a sugerir")
	}
}
