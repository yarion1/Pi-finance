package core

import "testing"

func TestFormatarBRL(t *testing.T) {
	casos := map[Centavos]string{
		0:            "R$ 0,00",
		5:            "R$ 0,05",
		123456:       "R$ 1.234,56",
		-4500:        "-R$ 45,00",
		8100000:      "R$ 81.000,00",
		480000000:    "R$ 4.800.000,00",
		100000000000: "R$ 1.000.000.000,00",
	}
	for c, esperado := range casos {
		if got := FormatarBRL(c); got != esperado {
			t.Errorf("FormatarBRL(%d) = %q, esperado %q", c, got, esperado)
		}
	}
	if got := FormatarNumero(123456); got != "1.234,56" {
		t.Errorf("FormatarNumero = %q", got)
	}
}

func TestParseBRL(t *testing.T) {
	validos := map[string]Centavos{
		"1.234,56":    123456,
		"R$ 1.234,56": 123456,
		"R$ 45":       4500,
		"-45":         -4500,
		"-R$ 45,5":    -4550,
		"45,5":        4550,
		"0,99":        99,
		",99":         99,
		"1234.56":     123456,
		"1.234.567":   123456700,
		"81000":       8100000,
	}
	for s, esperado := range validos {
		got, err := ParseBRL(s)
		if err != nil || got != esperado {
			t.Errorf("ParseBRL(%q) = %d, %v; esperado %d", s, got, err, esperado)
		}
	}
	for _, s := range []string{"", "abc", "1,2,3", "1,234", "R$", "12a"} {
		if _, err := ParseBRL(s); err == nil {
			t.Errorf("ParseBRL(%q) deveria falhar", s)
		}
	}
}

func TestIdaEVolta(t *testing.T) {
	for _, c := range []Centavos{0, 1, 99, 100, 123456, -987654321} {
		got, err := ParseBRL(FormatarBRL(c))
		if err != nil || got != c {
			t.Errorf("ida e volta de %d: %d, %v", c, got, err)
		}
	}
}
