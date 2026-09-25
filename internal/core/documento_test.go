package core

import "testing"

func TestDocumentos(t *testing.T) {
	// números gerados para teste, sem dono
	if !CPFValido("52998224725") || !CPFValido(SoDigitos("529.982.247-25")) {
		t.Error("CPF válido recusado")
	}
	for _, c := range []string{"52998224724", "11111111111", "123", "5299822472a"} {
		if CPFValido(c) {
			t.Errorf("CPF %s aceito", c)
		}
	}
	if !CNPJValido("11222333000181") || !CNPJValido(SoDigitos("11.222.333/0001-81")) {
		t.Error("CNPJ válido recusado")
	}
	for _, c := range []string{"11222333000180", "00000000000000", "112223330001"} {
		if CNPJValido(c) {
			t.Errorf("CNPJ %s aceito", c)
		}
	}
	if MascararDocumento("52998224725") != "***.982.247-**" {
		t.Error(MascararDocumento("52998224725"))
	}
	if MascararDocumento("11222333000181") != "**.222.333/0001-**" {
		t.Error(MascararDocumento("11222333000181"))
	}
}
