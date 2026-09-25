package core

import (
	"regexp"
	"strconv"
	"strings"
	"time"
)

// DiaNoMes devolve o dia pedido no mês, limitado ao último dia (31 em fevereiro → 28/29).
// Aceita mês fora de 1..12 (normaliza, como time.Date).
func DiaNoMes(ano int, mes time.Month, dia int) time.Time {
	primeiro := time.Date(ano, mes, 1, 0, 0, 0, 0, time.UTC)
	ultimo := primeiro.AddDate(0, 1, -1).Day()
	if dia > ultimo {
		dia = ultimo
	}
	if dia < 1 {
		dia = 1
	}
	return time.Date(primeiro.Year(), primeiro.Month(), dia, 0, 0, 0, 0, time.UTC)
}

// SomarMeses soma n meses mantendo o dia (31/01 + 1 mês = 28/02).
func SomarMeses(t time.Time, n int) time.Time {
	return DiaNoMes(t.Year(), t.Month()+time.Month(n), t.Day())
}

// FaturaDaCompra diz em que fatura do cartão a compra entra. Compra no dia do
// fechamento ou depois já cai na fatura seguinte (por isso o "melhor dia de compra"
// é o dia do fechamento). O vencimento é no mesmo mês do fechamento se o dia for
// maior; senão, no mês seguinte.
func FaturaDaCompra(compra time.Time, diaFechamento, diaVencimento int) (fechamento, vencimento time.Time) {
	fechamento = DiaNoMes(compra.Year(), compra.Month(), diaFechamento)
	if !truncarDia(compra).Before(fechamento) {
		fechamento = DiaNoMes(compra.Year(), compra.Month()+1, diaFechamento)
	}
	mesVenc := fechamento.Month()
	if diaVencimento <= diaFechamento {
		mesVenc++
	}
	vencimento = DiaNoMes(fechamento.Year(), mesVenc, diaVencimento)
	return fechamento, vencimento
}

// FechamentoDaFatura é o fechamento da fatura que vence em vencimento.
func FechamentoDaFatura(vencimento time.Time, diaFechamento, diaVencimento int) time.Time {
	mes := vencimento.Month()
	if diaVencimento <= diaFechamento {
		mes--
	}
	return DiaNoMes(vencimento.Year(), mes, diaFechamento)
}

// DividirParcelas divide o total em n parcelas que somam exatamente o total; os
// centavos que sobram vão para as primeiras (como as lojas fazem).
func DividirParcelas(total Centavos, n int) []Centavos {
	if n < 1 {
		return nil
	}
	sinal := Centavos(1)
	if total < 0 {
		sinal, total = -1, -total
	}
	base, resto := total/Centavos(n), total%Centavos(n)
	out := make([]Centavos, n)
	for i := range out {
		out[i] = base
		if Centavos(i) < resto {
			out[i]++
		}
		out[i] *= sinal
	}
	return out
}

var (
	reParcelaPalavra = regexp.MustCompile(`(?i)\s*[-–]?\s*\bparc(?:ela)?\.?\s*(\d{1,2})\s*(?:/|de)\s*(\d{1,2})\b`)
	reParcelaParen   = regexp.MustCompile(`\s*\((\d{1,2})/(\d{1,2})\)`)
)

// ExtrairParcela reconhece "Loja - Parcela 3/12", "LOJA PARC 03/12" e "Loja (3/12)".
// Datas como "12/09" soltas não contam: é preciso "parcela"/"parc" ou parênteses.
func ExtrairParcela(descricao string) (base string, n, total int, ok bool) {
	for _, re := range []*regexp.Regexp{reParcelaPalavra, reParcelaParen} {
		m := re.FindStringSubmatchIndex(descricao)
		if m == nil {
			continue
		}
		n, _ = strconv.Atoi(descricao[m[2]:m[3]])
		total, _ = strconv.Atoi(descricao[m[4]:m[5]])
		if n < 1 || total < 2 || n > total || total > 72 {
			continue
		}
		base = strings.Trim(strings.TrimSpace(descricao[:m[0]]+" "+descricao[m[1]:]), " -–")
		return base, n, total, true
	}
	return descricao, 0, 0, false
}
