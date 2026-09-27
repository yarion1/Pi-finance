package core

import (
	"time"
)

// Séries acumuladas para comparar a carteira com os índices (tela Investimentos).

// SerieRetorno: retorno acumulado da carteira (TWR, a "cota") em cada data. valores[i] é
// o valor na data i; fluxos são os do investidor (compra negativa, venda e provento
// positivos), que entram na carteira com o sinal trocado no período em que acontecem.
func SerieRetorno(datas []time.Time, valores []Centavos, fluxos []Fluxo) []float64 {
	pontos := make([]PontoCarteira, len(datas))
	for i, d := range datas {
		pontos[i] = PontoCarteira{Data: d, Valor: valores[i]}
		if i == 0 {
			continue
		}
		for _, f := range fluxos {
			if f.Data.After(datas[i-1]) && !f.Data.After(d) {
				pontos[i].Fluxo -= f.Valor
			}
		}
	}
	_, cotas, err := TWR(pontos)
	out := make([]float64, len(datas))
	if err != nil {
		return out
	}
	for i, c := range cotas {
		out[i] = c - 1
	}
	return out
}

// CDIAcumulado desde a primeira data; nil onde a série do CDI não cobre (falta mais de uma
// semana de dados no começo ou no fim).
func CDIAcumulado(datas []time.Time, cdi []TaxaDia) []*float64 {
	out := make([]*float64, len(datas))
	if len(datas) == 0 || len(cdi) == 0 || cdi[0].Data.After(datas[0].AddDate(0, 0, 7)) {
		return out
	}
	ultimo := cdi[len(cdi)-1].Data
	fator, j := 1.0, 0
	for i, d := range datas {
		for j < len(cdi) && !cdi[j].Data.After(d) {
			if cdi[j].Data.After(datas[0]) {
				fator *= 1 + cdi[j].Taxa
			}
			j++
		}
		if d.After(ultimo.AddDate(0, 0, 7)) {
			continue
		}
		v := fator - 1
		out[i] = &v
	}
	return out
}

// IPCAAcumulado: produto dos meses depois do mês da primeira data até o mês de cada data
// (datas de fim de mês); nil quando falta algum mês (o IPCA sai com atraso).
func IPCAAcumulado(datas []time.Time, ipca map[string]float64) []*float64 {
	out := make([]*float64, len(datas))
	if len(datas) == 0 {
		return out
	}
	inicio := time.Date(datas[0].Year(), datas[0].Month(), 1, 0, 0, 0, 0, time.UTC)
	for i, d := range datas {
		fator := 1.0
		completo := true
		for m := inicio.AddDate(0, 1, 0); !m.After(d); m = m.AddDate(0, 1, 0) {
			t, ok := ipca[m.Format("2006-01")]
			if !ok {
				completo = false
				break
			}
			fator *= 1 + t
		}
		if completo {
			v := fator - 1
			out[i] = &v
		}
	}
	return out
}

// PrecoData: um fechamento.
type PrecoData struct {
	Data  time.Time
	Preco float64
}

// PrecoAcumulado (ex.: Ibovespa): último fechamento até cada data sobre o último até a
// primeira data; nil sem fechamento que sirva.
func PrecoAcumulado(datas []time.Time, precos []PrecoData) []*float64 {
	out := make([]*float64, len(datas))
	ate := func(d time.Time) (float64, bool) {
		p, ok := 0.0, false
		for _, x := range precos {
			if x.Data.After(d) {
				break
			}
			p, ok = x.Preco, true
		}
		return p, ok
	}
	if len(datas) == 0 {
		return out
	}
	base, ok := ate(datas[0])
	if !ok || base <= 0 {
		return out
	}
	for i, d := range datas {
		if p, ok := ate(d); ok {
			v := p/base - 1
			out[i] = &v
		}
	}
	return out
}
