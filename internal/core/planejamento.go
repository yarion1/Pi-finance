package core

import (
	"sort"
	"time"
)

// DiasNoMes de um mês.
func DiasNoMes(ano int, mes time.Month) int {
	return time.Date(ano, mes+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// RitmoIdeal é quanto do limite já "deveria" ter sido gasto até o dia, num ritmo
// uniforme (a marca na barra do orçamento).
func RitmoIdeal(limite Centavos, dia, diasNoMes int) Centavos {
	if diasNoMes <= 0 || dia <= 0 {
		return 0
	}
	if dia > diasNoMes {
		dia = diasNoMes
	}
	return arredondarDivisao(limite*Centavos(dia), Centavos(diasNoMes))
}

func arredondarDivisao(a, b Centavos) Centavos {
	if (a < 0) != (b < 0) {
		return (a - b/2) / b
	}
	return (a + b/2) / b
}

// SituacaoOrcamento: "estourado" (passou do limite), "acima_do_ritmo" (gastou mais
// que o ritmo ideal até hoje) ou "ok".
func SituacaoOrcamento(disponivel, gasto, ritmo Centavos) string {
	switch {
	case gasto > disponivel:
		return "estourado"
	case gasto > ritmo:
		return "acima_do_ritmo"
	default:
		return "ok"
	}
}

// AporteNecessario é quanto guardar por mês, a partir de agora, para chegar ao
// alvo na data (conta o mês atual). Sem data, ou já alcançada, devolve 0.
func AporteNecessario(alvo, atual Centavos, hoje time.Time, dataAlvo *time.Time) (aporte Centavos, meses int) {
	falta := alvo - atual
	if falta <= 0 || dataAlvo == nil {
		return 0, 0
	}
	meses = (dataAlvo.Year()-hoje.Year())*12 + int(dataAlvo.Month()-hoje.Month()) + 1
	if meses < 1 {
		meses = 1
	}
	return (falta + Centavos(meses) - 1) / Centavos(meses), meses
}

// DataPrevista: em que mês o alvo é alcançado guardando aporte por mês. Nil se o
// aporte não é positivo.
func DataPrevista(alvo, atual, aporte Centavos, hoje time.Time) *time.Time {
	falta := alvo - atual
	if falta <= 0 {
		d := truncarDia(hoje)
		return &d
	}
	if aporte <= 0 {
		return nil
	}
	meses := int((falta + aporte - 1) / aporte)
	if meses > 1200 {
		return nil
	}
	d := DiaNoMes(hoje.Year(), hoje.Month()+time.Month(meses-1), DiasNoMes(hoje.Year(), hoje.Month()+time.Month(meses-1)))
	return &d
}

// MesesCobertos: quantos meses de custo de vida um valor cobre, em décimos
// (45 = 4,5 meses). -1 quando não há custo de vida para comparar.
func MesesCobertos(valor, custoMensal Centavos) int {
	if custoMensal <= 0 {
		return -1
	}
	if valor <= 0 {
		return 0
	}
	return int(valor * 10 / custoMensal)
}

// Media arredondada de valores (custo de vida médio de N meses).
func Media(valores []Centavos) Centavos {
	if len(valores) == 0 {
		return 0
	}
	var s Centavos
	for _, v := range valores {
		s += v
	}
	return arredondarDivisao(s, Centavos(len(valores)))
}

// Ocorrencia de uma transação que talvez se repita.
type Ocorrencia struct {
	Data  time.Time
	Valor Centavos
}

// Recorrencia detectada numa série de ocorrências.
type Recorrencia struct {
	Frequencia    string    `json:"frequencia"` // "mensal" ou "anual"
	Dia           int       `json:"dia"`
	Valor         Centavos  `json:"valor_centavos"` // o último
	ValorAnterior Centavos  `json:"valor_anterior_centavos"`
	Proxima       time.Time `json:"proxima"`
	Ultima        time.Time `json:"ultima"`
	Subiu         bool      `json:"subiu"` // último valor ≥ 5 % maior que o anterior
}

func abs(c Centavos) Centavos {
	if c < 0 {
		return -c
	}
	return c
}

// DetectarRecorrencia olha as ocorrências de uma mesma descrição e decide se é
// conta fixa ou assinatura: mensal com pelo menos 3 meses diferentes, intervalos de
// 25 a 35 dias e valores até 15 % da mediana; ou anual com 2 cobranças a 350–380 dias.
func DetectarRecorrencia(ocs []Ocorrencia) (Recorrencia, bool) {
	if len(ocs) < 2 {
		return Recorrencia{}, false
	}
	s := append([]Ocorrencia(nil), ocs...)
	sort.Slice(s, func(i, j int) bool { return s[i].Data.Before(s[j].Data) })

	valores := make([]Centavos, len(s))
	for i, o := range s {
		valores[i] = abs(o.Valor)
	}
	ord := append([]Centavos(nil), valores...)
	sort.Slice(ord, func(i, j int) bool { return ord[i] < ord[j] })
	mediana := ord[len(ord)/2]
	for _, v := range valores {
		if mediana == 0 || abs(v-mediana)*100 > mediana*15 {
			return Recorrencia{}, false
		}
	}

	ultima, anterior := s[len(s)-1], s[len(s)-2]
	r := Recorrencia{Valor: ultima.Valor, ValorAnterior: anterior.Valor, Ultima: truncarDia(ultima.Data)}
	r.Subiu = abs(ultima.Valor)*100 >= abs(anterior.Valor)*105

	intervalos := func(min, max float64) bool {
		for i := 1; i < len(s); i++ {
			d := s[i].Data.Sub(s[i-1].Data).Hours() / 24
			if d < min || d > max {
				return false
			}
		}
		return true
	}
	meses := map[string]bool{}
	dias := make([]int, 0, len(s))
	for _, o := range s {
		meses[o.Data.Format("2006-01")] = true
		dias = append(dias, o.Data.Day())
	}
	sort.Ints(dias)
	r.Dia = dias[len(dias)/2]

	switch {
	case len(meses) >= 3 && intervalos(25, 35):
		r.Frequencia = "mensal"
		r.Proxima = DiaNoMes(ultima.Data.Year(), ultima.Data.Month()+1, r.Dia)
	case len(s) >= 2 && intervalos(350, 380):
		r.Frequencia = "anual"
		r.Proxima = DiaNoMes(ultima.Data.Year()+1, ultima.Data.Month(), r.Dia)
	default:
		return Recorrencia{}, false
	}
	return r, true
}

// ProximaOcorrencia avança uma recorrência conhecida até a primeira data depois de 'depois'.
func ProximaOcorrencia(ultima time.Time, frequencia string, dia int, depois time.Time) time.Time {
	d := truncarDia(ultima)
	for i := 0; i < 1000; i++ {
		switch frequencia {
		case "anual":
			d = DiaNoMes(d.Year()+1, d.Month(), dia)
		case "semanal":
			d = d.AddDate(0, 0, 7)
		default:
			d = DiaNoMes(d.Year(), d.Month()+1, dia)
		}
		if d.After(truncarDia(depois)) {
			return d
		}
	}
	return d
}
