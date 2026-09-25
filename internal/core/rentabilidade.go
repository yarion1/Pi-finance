package core

import (
	"errors"
	"math"
	"sort"
	"time"
)

// Taxas e retornos são razões (não dinheiro), então float64 basta: a exibição é em
// pontos percentuais com duas casas.

// PontoCarteira: valor de mercado no fim do dia e o fluxo do dia (aporte positivo,
// resgate ou provento recebido negativo), considerado no começo do dia.
type PontoCarteira struct {
	Data  time.Time
	Valor Centavos
	Fluxo Centavos
}

// ErrSemDados: série vazia ou sem valor para medir.
var ErrSemDados = errors.New("sem dados suficientes")

// TWR: retorno ponderado pelo tempo (a "cota" da carteira), que não depende de quando
// o dinheiro entrou. Encadeia os retornos diários r = V / (V_anterior + fluxo) − 1.
// Devolve também a série da cota (começa em 1) para comparar com índices.
func TWR(pontos []PontoCarteira) (float64, []float64, error) {
	if len(pontos) == 0 {
		return 0, nil, ErrSemDados
	}
	ps := append([]PontoCarteira(nil), pontos...)
	sort.SliceStable(ps, func(i, j int) bool { return ps[i].Data.Before(ps[j].Data) })
	cota := 1.0
	serie := make([]float64, len(ps))
	anterior := Centavos(0)
	for i, p := range ps {
		base := anterior + p.Fluxo
		if base > 0 {
			cota *= float64(p.Valor) / float64(base)
		}
		serie[i] = cota
		anterior = p.Valor
	}
	return cota - 1, serie, nil
}

// Fluxo para o XIRR, do ponto de vista do investidor: aporte negativo, resgate e valor
// final positivos.
type Fluxo struct {
	Data  time.Time
	Valor Centavos
}

func vpl(fluxos []Fluxo, taxa float64) (valor, derivada float64) {
	d0 := fluxos[0].Data
	for _, f := range fluxos {
		t := f.Data.Sub(d0).Hours() / 24 / 365
		fator := math.Pow(1+taxa, t)
		valor += float64(f.Valor) / fator
		derivada -= t * float64(f.Valor) / (fator * (1 + taxa))
	}
	return valor, derivada
}

// XIRR: taxa anual que zera o valor presente dos fluxos (convenção dias/365, a mesma da
// função XIRR das planilhas). Newton a partir de 10 %; se não convergir, bisseção.
func XIRR(fluxos []Fluxo) (float64, error) {
	fs := append([]Fluxo(nil), fluxos...)
	sort.SliceStable(fs, func(i, j int) bool { return fs[i].Data.Before(fs[j].Data) })
	var pos, neg bool
	for _, f := range fs {
		pos = pos || f.Valor > 0
		neg = neg || f.Valor < 0
	}
	if !pos || !neg {
		return 0, ErrSemDados
	}
	taxa := 0.1
	for range 100 {
		v, d := vpl(fs, taxa)
		if math.Abs(v) < 1e-7 {
			return taxa, nil
		}
		if d == 0 || math.IsNaN(d) {
			break
		}
		nova := taxa - v/d
		if nova <= -0.999999 || math.IsNaN(nova) || math.IsInf(nova, 0) {
			break
		}
		taxa = nova
	}
	// bisseção entre −99,9999 % e 10.000 % ao ano
	lo, hi := -0.999999, 100.0
	vlo, _ := vpl(fs, lo)
	vhi, _ := vpl(fs, hi)
	if (vlo > 0) == (vhi > 0) {
		// a raiz está fora do intervalo (perda ou ganho extremo): fica com a ponta mais próxima
		if math.Abs(vlo) < math.Abs(vhi) {
			return lo, nil
		}
		return hi, nil
	}
	for range 300 {
		meio := (lo + hi) / 2
		v, _ := vpl(fs, meio)
		if (v > 0) == (vlo > 0) {
			lo, vlo = meio, v
		} else {
			hi = meio
		}
	}
	return (lo + hi) / 2, nil
}

// NoPeriodo converte uma taxa anual para o retorno de um período de n dias corridos.
func NoPeriodo(anual float64, dias int) float64 {
	return math.Pow(1+anual, float64(dias)/365) - 1
}

// Acumular encadeia taxas de cada período (0,01 = 1 %) num retorno total.
func Acumular(taxas []float64) float64 {
	f := 1.0
	for _, t := range taxas {
		f *= 1 + t
	}
	return f - 1
}

// ---------------------------------------------------------------------------
// Renda fixa na curva
// ---------------------------------------------------------------------------

// TaxaDia: taxa de um dia útil (o CDI publicado pelo Banco Central, em fração: 0,000551).
type TaxaDia struct {
	Data time.Time
	Taxa float64
}

// Indexadores de renda fixa.
const (
	IdxPrefixado = "prefixado"
	IdxCDI       = "cdi"
	IdxIPCA      = "ipca"
)

// RendaFixa descreve um título comprado numa data.
type RendaFixa struct {
	Indexador string
	Taxa      float64 // prefixado e IPCA+: taxa anual (0,12 = 12 % a.a.); CDI: percentual (1,10 = 110 % do CDI)
	Aplicacao time.Time
	Principal Centavos
}

// ValorNaCurva do título na data. CDI: principal × Π(1 + CDI_dia × percentual) nos dias
// úteis depois da aplicação. Prefixado: (1 + taxa)^(du/252). IPCA+: IPCA de cada mês
// cheio (pro rata pelos dias corridos no mês incompleto, com o último IPCA conhecido)
// × (1 + taxa)^(du/252).
func ValorNaCurva(t RendaFixa, data time.Time, cdi []TaxaDia, ipcaMensal map[string]float64) Centavos {
	if !data.After(t.Aplicacao) {
		return t.Principal
	}
	fator := 1.0
	switch t.Indexador {
	case IdxCDI:
		for _, d := range cdi {
			if d.Data.After(t.Aplicacao) && !d.Data.After(data) {
				fator *= 1 + d.Taxa*t.Taxa
			}
		}
	case IdxPrefixado:
		fator = math.Pow(1+t.Taxa, float64(DiasUteis(t.Aplicacao, data))/252)
	case IdxIPCA:
		fator = fatorIPCA(t.Aplicacao, data, ipcaMensal) * math.Pow(1+t.Taxa, float64(DiasUteis(t.Aplicacao, data))/252)
	}
	return Centavos(math.Round(float64(t.Principal) * fator))
}

func fatorIPCA(de, ate time.Time, ipca map[string]float64) float64 {
	fator, ultimo := 1.0, 0.0
	for m := DiaNoMes(de.Year(), de.Month(), 1); !m.After(ate); m = m.AddDate(0, 1, 0) {
		taxa, ok := ipca[m.Format("2006-01")]
		if ok {
			ultimo = taxa
		} else {
			taxa = ultimo // mês ainda não divulgado: repete o último
		}
		diasMes := float64(DiasNoMes(m.Year(), m.Month()))
		inicio, fim := m, m.AddDate(0, 1, 0)
		if de.After(inicio) {
			inicio = de
		}
		if ate.Before(fim) {
			fim = ate
		}
		dias := fim.Sub(inicio).Hours() / 24
		if dias > 0 {
			fator *= math.Pow(1+taxa, dias/diasMes)
		}
	}
	return fator
}

// FaixaIR da tabela regressiva (regras_fiscais: ir_regressivo_renda_fixa).
type FaixaIR struct {
	AteDias  int    `json:"ate_dias"` // 0 = sem limite
	Aliquota string `json:"aliquota"`
}

// IRRendaFixa sobre o rendimento, pela tabela regressiva e pelos dias corridos desde a
// aplicação. Título isento (LCI, LCA, CRI, CRA, incentivadas) não paga.
func IRRendaFixa(rendimento Centavos, dias int, faixas []FaixaIR, isento bool) Centavos {
	if isento || rendimento <= 0 {
		return 0
	}
	for _, f := range faixas {
		if f.AteDias == 0 || dias <= f.AteDias {
			return aplicarAliquota(rendimento, f.Aliquota)
		}
	}
	return 0
}

// FluxosDasOperacoes monta os fluxos do XIRR de um ativo até a data: compra sai (com as
// taxas); venda, provento, juros e amortização entram (líquidos de taxas e IR retido); e o
// valor na data entra como se fosse resgatado.
func FluxosDasOperacoes(ops []Operacao, valorFinal Centavos, data time.Time) []Fluxo {
	var out []Fluxo
	for _, o := range ops {
		if o.Data.After(data) {
			continue
		}
		switch o.Tipo {
		case OpCompra:
			out = append(out, Fluxo{Data: o.Data, Valor: -(Valor(o.Quantidade, o.Preco) + o.Taxas)})
		case OpVenda:
			out = append(out, Fluxo{Data: o.Data, Valor: Valor(o.Quantidade, o.Preco) - o.Taxas - o.IRRetido})
		default:
			out = append(out, Fluxo{Data: o.Data, Valor: o.Valor - o.Taxas - o.IRRetido})
		}
	}
	if valorFinal > 0 {
		out = append(out, Fluxo{Data: data, Valor: valorFinal})
	}
	return out
}

// Anualizar: retorno de n dias corridos para a taxa equivalente ao ano (inverso de NoPeriodo).
func Anualizar(retorno float64, dias int) float64 {
	if dias <= 0 {
		return 0
	}
	return math.Pow(1+retorno, 365/float64(dias)) - 1
}
