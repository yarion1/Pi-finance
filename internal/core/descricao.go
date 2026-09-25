package core

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var tiraAcentos = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// NormalizarDescricao: minúsculas, sem acento, espaços simples. Base da deduplicação.
func NormalizarDescricao(s string) string {
	s, _, _ = transform.String(tiraAcentos, s)
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// ChaveHistorico tira números, datas e símbolos para achar descrições "parecidas"
// ("UBER *TRIP 8812" e "Uber *trip 1290" → "uber trip"), usada para categorizar
// pelo histórico.
func ChaveHistorico(s string) string {
	s = NormalizarDescricao(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case unicode.IsLetter(r):
			b.WriteRune(r)
		default:
			b.WriteRune(' ')
		}
	}
	palavras := strings.Fields(b.String())
	// palavras de uma letra costumam ser ruído (ex.: "*" virou espaço, "s a")
	out := palavras[:0]
	for _, p := range palavras {
		if len(p) > 1 {
			out = append(out, p)
		}
	}
	return strings.Join(out, " ")
}

// ChaveDedup identifica a transação dentro da conta: data, valor, descrição
// normalizada e a ordem entre linhas idênticas do mesmo arquivo (duas compras
// iguais no mesmo dia continuam sendo duas).
func ChaveDedup(data time.Time, valor Centavos, descricao string, ordem int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%s|%d", data.Format("2006-01-02"), valor, NormalizarDescricao(descricao), ordem)))
	return hex.EncodeToString(h[:16])
}

// ParseDecimalPonto lê valores com ponto decimal ("-45.9", "1,234.56", "1234").
func ParseDecimalPonto(s string) (Centavos, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", ""))
	if s == "" {
		return 0, ErrValorInvalido
	}
	if strings.Count(s, ".") > 1 {
		return 0, ErrValorInvalido
	}
	if i := strings.Index(s, "."); i >= 0 && len(s)-i-1 > 2 {
		// mais de 2 casas: arredonda meio para longe do zero, como os bancos
		inteiro, fracao := s[:i+1+2], s[i+3:]
		c, err := ParseBRL(strings.Replace(inteiro, ".", ",", 1))
		if err != nil || !soDigitos(fracao) {
			return 0, ErrValorInvalido
		}
		if fracao[0] >= '5' {
			if c < 0 || strings.HasPrefix(s, "-") {
				c--
			} else {
				c++
			}
		}
		return c, nil
	}
	return ParseBRL(strings.Replace(s, ".", ",", 1))
}
