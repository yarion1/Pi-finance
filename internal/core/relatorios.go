package core

import (
	"fmt"
	"time"
)

// MesDoRelatorio: o mês fechado mais recente (o relatório do mês sai no dia 1).
func MesDoRelatorio(hoje time.Time) time.Time {
	return time.Date(hoje.Year(), hoje.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -1, 0)
}

// HoraResumoSemana: a partir desta hora de domingo (São Paulo) o resumo da semana sai.
const HoraResumoSemana = 19

// SemanaDoResumo: segunda-feira da última semana (segunda a domingo) já encerrada; no
// domingo a partir das 19 h, a própria semana.
func SemanaDoResumo(agora time.Time) time.Time {
	dia := time.Date(agora.Year(), agora.Month(), agora.Day(), 0, 0, 0, 0, time.UTC)
	desdeSegunda := (int(dia.Weekday()) + 6) % 7 // segunda = 0 ... domingo = 6
	segunda := dia.AddDate(0, 0, -desdeSegunda)
	if desdeSegunda == 6 && agora.Hour() >= HoraResumoSemana {
		return segunda
	}
	return segunda.AddDate(0, 0, -7)
}

// EntradaAcoes: os números do mês que viram sugestões.
type EntradaAcoes struct {
	Receitas, Gastos    Centavos
	CategoriaQueSubiu   string   // maior alta contra a média de 3 meses
	AltaCategoria       Centavos // quanto subiu
	AltaPercentual      float64
	Estouradas          []string // categorias que passaram do orçamento
	Reserva, CustoMedio Centavos // dinheiro em conta e custo de vida médio
	Assinaturas         Centavos // total mensal das assinaturas ativas
	SemCategoria        int
}

// Limites das sugestões.
const (
	AltaMinima           = Centavos(100_00) // uma categoria "subiu" a partir disso...
	AltaMinimaPercentual = 0.2              // ...e de 20 % sobre a média
	MesesReservaMinima   = 3
)

// SugerirAcoes: até três ações, das mais urgentes para as de rotina.
func SugerirAcoes(e EntradaAcoes) []string {
	var out []string
	saldo := e.Receitas - e.Gastos
	if saldo < 0 {
		out = append(out, fmt.Sprintf("Os gastos passaram as receitas em %s: veja o que dá para cortar no mês que vem.", FormatarBRL(-saldo)))
	}
	if e.CategoriaQueSubiu != "" && e.AltaCategoria >= AltaMinima && e.AltaPercentual >= AltaMinimaPercentual {
		out = append(out, fmt.Sprintf("%s subiu %s (%.0f %%) contra a média dos 3 meses anteriores: vale revisar.",
			e.CategoriaQueSubiu, FormatarBRL(e.AltaCategoria), e.AltaPercentual*100))
	}
	if n := len(e.Estouradas); n == 1 {
		out = append(out, fmt.Sprintf("%s passou do orçamento: ajuste o limite ou o gasto.", e.Estouradas[0]))
	} else if n > 1 {
		out = append(out, fmt.Sprintf("%d categorias passaram do orçamento, a começar por %s.", n, e.Estouradas[0]))
	}
	if e.CustoMedio > 0 {
		if decimos := MesesCobertos(e.Reserva, e.CustoMedio); decimos < MesesReservaMinima*10 {
			unidade := "meses"
			if decimos < 20 {
				unidade = "mês"
			}
			out = append(out, fmt.Sprintf("O dinheiro em conta cobre %d,%d %s de custo de vida; a reserva de emergência costuma ser de 6 meses.",
				decimos/10, decimos%10, unidade))
		}
	}
	if saldo > 0 {
		out = append(out, fmt.Sprintf("Sobraram %s: dá para investir ou reforçar uma meta.", FormatarBRL(saldo)))
	}
	if e.Assinaturas > 0 {
		out = append(out, fmt.Sprintf("As assinaturas somam %s por mês: confira se usa todas.", FormatarBRL(e.Assinaturas)))
	}
	if e.SemCategoria == 1 {
		out = append(out, "1 transação ficou sem categoria; categorizar deixa os relatórios mais certos.")
	} else if e.SemCategoria > 1 {
		out = append(out, fmt.Sprintf("%d transações ficaram sem categoria; categorizar deixa os relatórios mais certos.", e.SemCategoria))
	}
	if len(out) > 3 {
		out = out[:3]
	}
	return out
}
