package core

import (
	"math/big"
	"testing"
)

// Casos de borda para manter o pacote 100 % coberto.
func TestBordas(t *testing.T) {
	if arredondar(big.NewFloat(-2.5)) != -3 || arredondar(big.NewFloat(-2.4)) != -2 || arredondar(big.NewFloat(2.5)) != 3 {
		t.Error("arredondar metade para longe do zero")
	}
	if DiaNoMes(2026, 9, 0).Day() != 1 {
		t.Error("dia 0 vira 1")
	}
	if v, err := ParseBRL("R$-45,00"); err != nil || v != -4500 {
		t.Error("sinal depois do R$")
	}
	if _, err := ParseBRL("99999999999999999999"); err == nil {
		t.Error("estouro de int64")
	}
	if restoParaDigito(11) != '0' || restoParaDigito(12) != '0' || restoParaDigito(13) != '9' {
		t.Error("resto menor que 2 vira dígito 0")
	}
	if MascararDocumento("123") != "***" {
		t.Error("documento de tamanho estranho")
	}
	if RitmoIdeal(-90000, 10, 30) != -30000 || arredondarDivisao(-5, 2) != -3 {
		t.Error("divisão negativa arredonda para longe do zero")
	}
	if DataPrevista(1_000_000_00, 0, 1, dia("2026-01-01")) != nil {
		t.Error("mais de 100 anos não é previsão")
	}
	if _, ok := DetectarRecorrencia([]Ocorrencia{{dia("2026-01-01"), 0}, {dia("2026-02-01"), 0}, {dia("2026-03-01"), 0}}); ok {
		t.Error("valor zero não é recorrência")
	}
	if _, ok := DetectarRecorrencia([]Ocorrencia{{dia("2026-01-01"), -10}}); ok {
		t.Error("uma ocorrência só")
	}
	longe := ProximaOcorrencia(dia("2026-01-01"), "mensal", 1, dia("2200-01-01"))
	if longe.Year() < 2100 {
		t.Error("para depois de 1000 passos")
	}
}
