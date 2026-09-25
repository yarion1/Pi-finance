package core

import (
	"errors"
	"testing"
	"time"
)

func dia(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}
	return t
}

func ptr(t time.Time) *time.Time { return &t }

func TestRegraVigente(t *testing.T) {
	regras := []Regra{
		{Chave: "teto_mei", Valor: []byte(`{"anual_centavos":8100000}`), VigenteDe: dia("2018-01-01"), VigenteAte: ptr(dia("2026-12-31"))},
		{Chave: "teto_mei", Valor: []byte(`{"anual_centavos":15000000}`), VigenteDe: dia("2027-01-01")},
		{Chave: "salario_minimo", Valor: []byte(`{"centavos":162100}`), VigenteDe: dia("2026-01-01")},
	}

	r, err := RegraVigente(regras, "teto_mei", dia("2026-09-25"))
	if err != nil || string(r.Valor) != `{"anual_centavos":8100000}` {
		t.Fatalf("2026: %s, %v", r.Valor, err)
	}
	r, err = RegraVigente(regras, "teto_mei", dia("2026-12-31"))
	if err != nil || string(r.Valor) != `{"anual_centavos":8100000}` {
		t.Fatalf("último dia de vigência: %s, %v", r.Valor, err)
	}
	r, err = RegraVigente(regras, "teto_mei", time.Date(2027, 1, 1, 23, 59, 0, 0, time.UTC))
	if err != nil || string(r.Valor) != `{"anual_centavos":15000000}` {
		t.Fatalf("2027: %s, %v", r.Valor, err)
	}
	if _, err := RegraVigente(regras, "salario_minimo", dia("2025-12-31")); !errors.Is(err, ErrSemRegraVigente) {
		t.Fatalf("antes da vigência: %v", err)
	}
	if _, err := RegraVigente(regras, "inexistente", dia("2026-01-01")); !errors.Is(err, ErrSemRegraVigente) {
		t.Fatalf("chave inexistente: %v", err)
	}

	sobrepostas := append(regras, Regra{Chave: "salario_minimo", VigenteDe: dia("2026-06-01")})
	if _, err := RegraVigente(sobrepostas, "salario_minimo", dia("2026-07-01")); !errors.Is(err, ErrRegrasSobrepostas) {
		t.Fatalf("sobreposição: %v", err)
	}
}
