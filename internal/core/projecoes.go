package core

import (
	"math"
	"math/rand/v2"
	"slices"
	"time"
)

// Projeções da seção 6 da SPEC. Estatística (média, desvio, cenários) é feita em float64
// e arredondada para centavos na saída; o dinheiro que entra e sai continua em Centavos.

func paraCentavos(f float64) Centavos { return Centavos(math.Round(f)) }

// DesvioPadrao amostral dos valores (0 com menos de dois).
func DesvioPadrao(vs []Centavos) Centavos {
	if len(vs) < 2 {
		return 0
	}
	var media float64
	for _, v := range vs {
		media += float64(v)
	}
	media /= float64(len(vs))
	var soma float64
	for _, v := range vs {
		d := float64(v) - media
		soma += d * d
	}
	return paraCentavos(math.Sqrt(soma / float64(len(vs)-1)))
}

// chaveMes "2026-09".
func chaveMes(t time.Time) string { return t.Format("2006-01") }

// VariavelDoMes: gasto variável esperado para o mês, pelos meses anteriores. A média dos
// três últimos meses fechados antes de hoje; se houver o mesmo mês do ano passado, ele
// pesa metade (sazonalidade: dezembro, material escolar, IPVA...).
func VariavelDoMes(historico map[string]Centavos, mes, hoje time.Time) Centavos {
	base := time.Date(hoje.Year(), hoje.Month(), 1, 0, 0, 0, 0, time.UTC)
	var ultimos []Centavos
	for i := 1; i <= 3; i++ {
		if v, ok := historico[chaveMes(base.AddDate(0, -i, 0))]; ok {
			ultimos = append(ultimos, v)
		}
	}
	if len(ultimos) == 0 {
		return 0
	}
	media := Media(ultimos)
	if v, ok := historico[chaveMes(time.Date(mes.Year()-1, mes.Month(), 1, 0, 0, 0, 0, time.UTC))]; ok {
		return (media + v) / 2
	}
	return media
}

// EntradaFluxo da projeção de caixa.
type EntradaFluxo struct {
	Hoje         time.Time
	Dias         int
	SaldoInicial Centavos
	Conhecidos   map[string]Centavos // "2026-09-30" → soma do que entra e sai no dia (recorrências, faturas, parcelas...)
	Variaveis    map[string]Centavos // "2026-05" → gasto variável (positivo) de cada mês passado
}

// FluxoDia: saldo projetado com a faixa de 80 % de confiança.
type FluxoDia struct {
	Data  time.Time `json:"data"`
	Base  Centavos  `json:"base_centavos"`
	Baixo Centavos  `json:"baixo_centavos"`
	Alto  Centavos  `json:"alto_centavos"`
}

// z da faixa de 80 % (percentis 10 e 90 da normal).
const z80 = 1.2815515655446004

// ProjetarFluxo: saldo de hoje + o que já se sabe (dia a dia) − gasto variável esperado
// espalhado pelos dias do mês. A faixa cresce com a raiz do tempo, pelo desvio dos
// gastos variáveis mensais.
func ProjetarFluxo(e EntradaFluxo) []FluxoDia {
	var meses []Centavos
	for _, v := range e.Variaveis {
		meses = append(meses, v)
	}
	desvio := float64(DesvioPadrao(meses))
	saldo := float64(e.SaldoInicial)
	out := make([]FluxoDia, 0, e.Dias+1)
	for i := 0; i <= e.Dias; i++ {
		d := e.Hoje.AddDate(0, 0, i)
		saldo += float64(e.Conhecidos[d.Format("2006-01-02")])
		if i > 0 {
			saldo -= float64(VariavelDoMes(e.Variaveis, d, e.Hoje)) / float64(DiasNoMes(d.Year(), d.Month()))
		}
		faixa := z80 * desvio * math.Sqrt(float64(i)/30)
		out = append(out, FluxoDia{Data: d, Base: paraCentavos(saldo), Baixo: paraCentavos(saldo - faixa), Alto: paraCentavos(saldo + faixa)})
	}
	return out
}

// PremissaClasse: retorno real esperado e volatilidade ao ano de uma classe de ativo.
type PremissaClasse struct {
	Retorno      float64 `json:"retorno_aa"`
	Volatilidade float64 `json:"volatilidade_aa"`
}

// PremissasPadrao: retornos reais (acima da inflação) e volatilidades de longo prazo,
// aproximados do histórico brasileiro; a tela deixa mudar.
var PremissasPadrao = map[string]PremissaClasse{
	"renda_fixa":  {0.045, 0.02},
	"tesouro":     {0.050, 0.05},
	"fundo":       {0.040, 0.05},
	"previdencia": {0.040, 0.05},
	"acao":        {0.070, 0.25},
	"etf":         {0.070, 0.22},
	"bdr":         {0.060, 0.20},
	"exterior":    {0.060, 0.18},
	"fii":         {0.060, 0.15},
	"cripto":      {0.080, 0.70},
	"outro":       {0.030, 0.10},
	"caixa":       {0.020, 0.01}, // contas: rende pouco acima da inflação
}

// ClasseMC: quanto há hoje numa classe e a parte do aporte mensal que vai para ela.
type ClasseMC struct {
	Classe     string   `json:"classe"`
	Valor      Centavos `json:"valor_centavos"`
	PesoAporte float64  `json:"peso_aporte"`
	PremissaClasse
}

// EntradaMC da simulação de Monte Carlo.
type EntradaMC struct {
	Classes      []ClasseMC
	AporteMensal Centavos
	Anos         int
	Cenarios     int
	Alvo         Centavos // patrimônio que cobre o custo de vida (0: sem alvo)
	Semente      uint64
}

// PontoMC: percentis do patrimônio no fim de cada ano, em dinheiro de hoje.
type PontoMC struct {
	Ano        int      `json:"ano"`
	P10        Centavos `json:"p10_centavos"`
	P50        Centavos `json:"p50_centavos"`
	P90        Centavos `json:"p90_centavos"`
	ChanceAlvo float64  `json:"chance_alvo"` // fração dos cenários que chegaram ao alvo
}

// MonteCarlo: cada cenário anda mês a mês; cada classe tem retorno lognormal
// independente com o retorno esperado e a volatilidade da premissa; o aporte entra no
// começo do mês, dividido pelos pesos. Simplificação: classes sem correlação.
func MonteCarlo(e EntradaMC) []PontoMC {
	if e.Anos < 1 || e.Cenarios < 1 || len(e.Classes) == 0 {
		return nil
	}
	gerador := rand.New(rand.NewPCG(e.Semente, e.Semente^0x9e3779b97f4a7c15))
	type passo struct{ mu, sigma, peso float64 }
	passos := make([]passo, len(e.Classes))
	var somaPesos float64
	for _, c := range e.Classes {
		somaPesos += c.PesoAporte
	}
	for i, c := range e.Classes {
		s := c.Volatilidade / math.Sqrt(12)
		peso := 1 / float64(len(e.Classes))
		if somaPesos > 0 {
			peso = c.PesoAporte / somaPesos
		}
		passos[i] = passo{mu: (math.Log1p(c.Retorno) - c.Volatilidade*c.Volatilidade/2) / 12, sigma: s, peso: peso}
	}
	fins := make([][]float64, e.Anos) // [ano][cenário]
	for a := range fins {
		fins[a] = make([]float64, e.Cenarios)
	}
	valores := make([]float64, len(e.Classes))
	aporte := float64(e.AporteMensal)
	for c := range e.Cenarios {
		for i, cl := range e.Classes {
			valores[i] = float64(cl.Valor)
		}
		for m := 1; m <= e.Anos*12; m++ {
			total := 0.0
			for i, p := range passos {
				valores[i] = (valores[i] + aporte*p.peso) * math.Exp(p.mu+p.sigma*gerador.NormFloat64())
				total += valores[i]
			}
			if m%12 == 0 {
				fins[m/12-1][c] = total
			}
		}
	}
	out := make([]PontoMC, e.Anos)
	for a, vs := range fins {
		slices.Sort(vs)
		p := PontoMC{Ano: a + 1, P10: paraCentavos(percentil(vs, 0.10)), P50: paraCentavos(percentil(vs, 0.50)),
			P90: paraCentavos(percentil(vs, 0.90))}
		if e.Alvo > 0 {
			i, _ := slices.BinarySearch(vs, float64(e.Alvo))
			p.ChanceAlvo = float64(len(vs)-i) / float64(len(vs))
		}
		out[a] = p
	}
	return out
}

// percentil de valores ordenados (interpolação linear).
func percentil(ordenados []float64, q float64) float64 {
	pos := q * float64(len(ordenados)-1)
	i := int(pos)
	if i+1 >= len(ordenados) {
		return ordenados[len(ordenados)-1]
	}
	return ordenados[i] + (pos-float64(i))*(ordenados[i+1]-ordenados[i])
}

// AlvoIndependencia: patrimônio cuja retirada anual (taxa segura, ex.: 4 %) cobre o custo
// de vida mensal.
func AlvoIndependencia(custoMensal Centavos, taxaRetirada float64) Centavos {
	if taxaRetirada <= 0 {
		return 0
	}
	return paraCentavos(float64(custoMensal) * 12 / taxaRetirada)
}

// MesesParaAlvo: com retorno real anual fixo e aporte mensal, quantos meses até o
// patrimônio chegar ao alvo (-1 se não chega em 100 anos).
func MesesParaAlvo(patrimonio, aporteMensal, alvo Centavos, retornoReal float64) int {
	if patrimonio >= alvo {
		return 0
	}
	r := math.Pow(1+retornoReal, 1.0/12) - 1
	v := float64(patrimonio)
	for m := 1; m <= 1200; m++ {
		v = (v + float64(aporteMensal)) * (1 + r)
		if v >= float64(alvo) {
			return m
		}
	}
	return -1
}
