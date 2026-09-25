package importadores

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/yarion1/pi-finance/internal/core"
)

var (
	reTransacao = regexp.MustCompile(`(?is)<STMTTRN>(.*?)</STMTTRN>`)
	reSaldo     = regexp.MustCompile(`(?is)<LEDGERBAL>(.*?)</LEDGERBAL>`)
)

// valorTag lê <TAG>valor tanto do OFX 1.x (SGML, sem fechamento) quanto do 2.x (XML).
func valorTag(bloco, tag string) string {
	re := regexp.MustCompile(`(?i)<` + tag + `>([^<\r\n]*)`)
	m := re.FindStringSubmatch(bloco)
	if m == nil {
		return ""
	}
	return strings.TrimSpace(m[1])
}

// dataOFX: "20260915", "20260915120000", "20260915120000[-3:BRT]".
func dataOFX(s string) (time.Time, error) {
	if len(s) < 8 {
		return time.Time{}, fmt.Errorf("data OFX inválida: %q", s)
	}
	t, err := time.Parse("20060102", s[:8])
	if err != nil {
		return time.Time{}, fmt.Errorf("data OFX inválida: %q", s)
	}
	return t, nil
}

// valorOFX aceita ponto decimal (padrão) e vírgula (alguns bancos brasileiros).
func valorOFX(s string) (core.Centavos, error) {
	s = strings.TrimSpace(s)
	if strings.Contains(s, ",") && !strings.Contains(s, ".") {
		return core.ParseBRL(s)
	}
	return core.ParseDecimalPonto(s)
}

func lerOFX(texto string) (Resultado, error) {
	r := Resultado{Formato: FormatoOFX, Moeda: strings.ToUpper(valorTag(texto, "CURDEF"))}
	if r.Moeda == "" {
		r.Moeda = "BRL"
	}
	blocos := reTransacao.FindAllStringSubmatch(texto, -1)
	if len(blocos) == 0 && !strings.Contains(strings.ToUpper(texto), "<BANKTRANLIST>") {
		return r, fmt.Errorf("OFX sem lista de transações")
	}
	for i, b := range blocos {
		bloco := b[1]
		data, err := dataOFX(valorTag(bloco, "DTPOSTED"))
		if err != nil {
			return r, fmt.Errorf("transação %d: %w", i+1, err)
		}
		valor, err := valorOFX(valorTag(bloco, "TRNAMT"))
		if err != nil {
			return r, fmt.Errorf("transação %d: valor inválido %q", i+1, valorTag(bloco, "TRNAMT"))
		}
		r.Linhas = append(r.Linhas, Linha{
			Data:      data,
			Descricao: descricaoOFX(valorTag(bloco, "NAME"), valorTag(bloco, "MEMO")),
			Valor:     valor,
			IDExterno: valorTag(bloco, "FITID"),
		})
	}
	if s := reSaldo.FindStringSubmatch(texto); s != nil {
		if v, err := valorOFX(valorTag(s[1], "BALAMT")); err == nil {
			r.SaldoFinal = &v
		}
		if d, err := dataOFX(valorTag(s[1], "DTASOF")); err == nil {
			r.SaldoEm = &d
		}
	}
	return r, nil
}

func descricaoOFX(nome, memo string) string {
	switch {
	case memo == "":
		return nome
	case nome == "" || strings.Contains(strings.ToLower(memo), strings.ToLower(nome)):
		return memo
	default:
		return nome + " - " + memo
	}
}
