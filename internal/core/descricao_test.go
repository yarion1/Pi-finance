package core

import (
	"testing"
	"time"
)

func TestNormalizarDescricao(t *testing.T) {
	casos := map[string]string{
		"  PAGAMENTO   Recebido ": "pagamento recebido",
		"Padaria São João":        "padaria sao joao",
		"UBER *TRIP 8812":         "uber *trip 8812",
	}
	for e, s := range casos {
		if got := NormalizarDescricao(e); got != s {
			t.Errorf("%q → %q, esperado %q", e, got, s)
		}
	}
}

func TestChaveHistorico(t *testing.T) {
	if ChaveHistorico("UBER *TRIP 8812") != "uber trip" || ChaveHistorico("Uber *trip 1290 12/09") != "uber trip" {
		t.Errorf("uber: %q", ChaveHistorico("UBER *TRIP 8812"))
	}
	if ChaveHistorico("Pix enviado - João S. da Silva") != "pix enviado joao da silva" {
		t.Errorf("pix: %q", ChaveHistorico("Pix enviado - João S. da Silva"))
	}
}

func TestChaveDedup(t *testing.T) {
	d := time.Date(2026, 9, 10, 0, 0, 0, 0, time.UTC)
	a := ChaveDedup(d, -4590, "Padaria  São João", 1)
	if a != ChaveDedup(d.Add(13*time.Hour), -4590, "PADARIA SAO JOAO", 1) {
		t.Error("mesma transação (horário e caixa diferentes) deveria dar a mesma chave")
	}
	if a == ChaveDedup(d, -4590, "Padaria São João", 2) {
		t.Error("segunda compra igual no dia precisa de outra chave")
	}
	if a == ChaveDedup(d, -4591, "Padaria São João", 1) || a == ChaveDedup(d.AddDate(0, 0, 1), -4590, "Padaria São João", 1) {
		t.Error("valor ou data diferentes precisam de outra chave")
	}
}

func TestParseDecimalPonto(t *testing.T) {
	validos := map[string]Centavos{"-45.9": -4590, "1,234.56": 123456, "1234": 123400, "0.01": 1, "-0.5": -50, "10.125": 1013, "-10.125": -1013, "3.004": 300}
	for s, quer := range validos {
		if got, err := ParseDecimalPonto(s); err != nil || got != quer {
			t.Errorf("%q → %d, %v; esperado %d", s, got, err, quer)
		}
	}
	for _, s := range []string{"", "1.2.3", "abc", "1.2x5"} {
		if _, err := ParseDecimalPonto(s); err == nil {
			t.Errorf("%q deveria falhar", s)
		}
	}
}
