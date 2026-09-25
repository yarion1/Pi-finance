// Package core reúne cálculos puros (sem banco nem rede). Tudo aqui é testado.
package core

import (
	"errors"
	"strconv"
	"strings"
)

// Centavos é um valor monetário em centavos. Dinheiro nunca passa por float.
type Centavos int64

// FormatarBRL formata no padrão brasileiro: R$ 1.234,56 e -R$ 45,00.
func FormatarBRL(c Centavos) string {
	return formatar(c, "R$ ")
}

// FormatarNumero formata sem símbolo: 1.234,56.
func FormatarNumero(c Centavos) string {
	return formatar(c, "")
}

func formatar(c Centavos, simbolo string) string {
	negativo := c < 0
	v := uint64(c)
	if negativo {
		v = uint64(-c)
	}
	inteiro := strconv.FormatUint(v/100, 10)
	var b strings.Builder
	if negativo {
		b.WriteByte('-')
	}
	b.WriteString(simbolo)
	for i, r := range inteiro {
		if i > 0 && (len(inteiro)-i)%3 == 0 {
			b.WriteByte('.')
		}
		b.WriteRune(r)
	}
	b.WriteByte(',')
	centavos := v % 100
	b.WriteByte(byte('0' + centavos/10))
	b.WriteByte(byte('0' + centavos%10))
	return b.String()
}

// ErrValorInvalido indica texto que não é um valor em reais.
var ErrValorInvalido = errors.New("valor inválido")

// ParseBRL lê "1.234,56", "R$ 1.234,56", "-45", "45,5" ou "1234.56" (ponto decimal
// só quando não há vírgula e há no máximo 2 casas depois do último ponto).
func ParseBRL(s string) (Centavos, error) {
	s = strings.TrimSpace(s)
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, " ", "")
	negativo := false
	if strings.HasPrefix(s, "-") {
		negativo, s = true, s[1:]
	}
	s = strings.TrimPrefix(s, "R$")
	if strings.HasPrefix(s, "-") {
		negativo, s = true, s[1:]
	}
	if s == "" {
		return 0, ErrValorInvalido
	}

	var inteiro, fracao string
	switch {
	case strings.Contains(s, ","):
		partes := strings.Split(s, ",")
		if len(partes) != 2 {
			return 0, ErrValorInvalido
		}
		inteiro, fracao = strings.ReplaceAll(partes[0], ".", ""), partes[1]
	case strings.Count(s, ".") == 1 && len(s)-strings.LastIndex(s, ".")-1 <= 2:
		i := strings.LastIndex(s, ".")
		inteiro, fracao = s[:i], s[i+1:]
	default:
		inteiro = strings.ReplaceAll(s, ".", "")
	}
	if inteiro == "" {
		inteiro = "0"
	}
	if len(fracao) > 2 || !soDigitos(inteiro) || !soDigitos(fracao) {
		return 0, ErrValorInvalido
	}
	for len(fracao) < 2 {
		fracao += "0"
	}
	n, err := strconv.ParseInt(inteiro+fracao, 10, 64)
	if err != nil {
		return 0, ErrValorInvalido
	}
	if negativo {
		n = -n
	}
	return Centavos(n), nil
}

func soDigitos(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
