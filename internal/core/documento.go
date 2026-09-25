package core

import "strings"

// SoDigitos remove pontuação de CPF/CNPJ.
func SoDigitos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CPFValido confere os dígitos verificadores (entrada só com dígitos).
func CPFValido(cpf string) bool {
	if len(cpf) != 11 || !soDigitos(cpf) || repetido(cpf) {
		return false
	}
	return digitoVerificador(cpf[:9], 10) == cpf[9] && digitoVerificador(cpf[:10], 11) == cpf[10]
}

// CNPJValido confere os dígitos verificadores (entrada só com dígitos).
func CNPJValido(cnpj string) bool {
	if len(cnpj) != 14 || !soDigitos(cnpj) || repetido(cnpj) {
		return false
	}
	pesos1 := []int{5, 4, 3, 2, 9, 8, 7, 6, 5, 4, 3, 2}
	pesos2 := append([]int{6}, pesos1...)
	return digitoPesos(cnpj[:12], pesos1) == cnpj[12] && digitoPesos(cnpj[:13], pesos2) == cnpj[13]
}

func repetido(s string) bool { return strings.Count(s, s[:1]) == len(s) }

func digitoVerificador(base string, pesoInicial int) byte {
	soma := 0
	for i, r := range base {
		soma += int(r-'0') * (pesoInicial - i)
	}
	return restoParaDigito(soma)
}

func digitoPesos(base string, pesos []int) byte {
	soma := 0
	for i, r := range base {
		soma += int(r-'0') * pesos[i]
	}
	return restoParaDigito(soma)
}

func restoParaDigito(soma int) byte {
	resto := soma % 11
	if resto < 2 {
		return '0'
	}
	return byte('0' + 11 - resto)
}

// MascararDocumento mostra só o miolo: ***.456.789-** e **.345.678/0001-**.
func MascararDocumento(doc string) string {
	switch len(doc) {
	case 11:
		return "***." + doc[3:6] + "." + doc[6:9] + "-**"
	case 14:
		return "**." + doc[2:5] + "." + doc[5:8] + "/" + doc[8:12] + "-**"
	default:
		return "***"
	}
}
