package core

import (
	"math/big"
	"time"
)

// MEI (SPEC §4): teto, faixas de alerta, projeção, DAS-MEI e lucro isento no IRPF.

// Situações do faturamento do MEI contra o teto.
const (
	MEINormal         = "normal"           // abaixo de 70 %
	MEIAtencao        = "atencao"          // 70 % a 90 %
	MEIPlanejar       = "planejar"         // 90 % a 100 %: planejar a migração
	MEIViraME         = "vira_me"          // 100 % a 120 %: vira ME em janeiro
	MEIDesenquadrado  = "desenquadramento" // acima de 120 %: desenquadramento retroativo
	atividadeServicos = "servicos"
	atividadeComercio = "comercio"
	atividadeAmbos    = "ambos"
)

// TetoMEI do ano: no ano de abertura, o proporcional × meses de atividade (do mês de
// abertura a dezembro); nos outros, o anual.
func TetoMEI(abertura *time.Time, ano int, anual, mensalProporcional Centavos) Centavos {
	if abertura == nil || abertura.Year() != ano {
		return anual
	}
	return mensalProporcional * Centavos(13-int(abertura.Month()))
}

// SituacaoMEI pelas faixas de alerta; tolerancia é o excesso que só muda o regime em
// janeiro ("0.20").
func SituacaoMEI(faturado, teto Centavos, tolerancia string) string {
	if teto <= 0 {
		return MEINormal
	}
	f := big.NewRat(int64(faturado), int64(teto))
	tol, ok := rat(tolerancia)
	if !ok {
		tol = new(big.Rat)
	}
	switch {
	case f.Cmp(new(big.Rat).Add(big.NewRat(1, 1), tol)) > 0:
		return MEIDesenquadrado
	case f.Cmp(big.NewRat(1, 1)) > 0:
		return MEIViraME
	case f.Cmp(big.NewRat(9, 10)) >= 0:
		return MEIPlanejar
	case f.Cmp(big.NewRat(7, 10)) >= 0:
		return MEIAtencao
	default:
		return MEINormal
	}
}

// ProjecaoMEI no ritmo do ano até hoje.
type ProjecaoMEI struct {
	Faturado       Centavos   `json:"faturado_centavos"`
	Teto           Centavos   `json:"teto_centavos"`
	Falta          Centavos   `json:"falta_centavos"`        // teto − faturado (≥ 0)
	MediaMaxima    Centavos   `json:"media_maxima_centavos"` // por mês até dezembro, contando o atual
	ProjecaoAno    Centavos   `json:"projecao_ano_centavos"` // no ritmo diário atual, até 31/12
	DataTeto       *time.Time `json:"data_teto"`             // quando bate o teto nesse ritmo (nil: não bate no ano)
	Percentual     float64    `json:"percentual"`            // faturado ÷ teto
	MesesRestantes int        `json:"meses_restantes"`       // incluindo o atual
	Inicio         time.Time  `json:"inicio"`                // 1º/jan ou a abertura
}

// ProjetarMEI: ritmo = faturado ÷ dias corridos desde o início do ano (ou da abertura)
// até hoje, inclusive.
func ProjetarMEI(faturado, teto Centavos, hoje time.Time, abertura *time.Time) ProjecaoMEI {
	inicio := time.Date(hoje.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	if abertura != nil && abertura.Year() == hoje.Year() && abertura.After(inicio) {
		inicio = time.Date(abertura.Year(), abertura.Month(), abertura.Day(), 0, 0, 0, 0, time.UTC)
	}
	hoje = time.Date(hoje.Year(), hoje.Month(), hoje.Day(), 0, 0, 0, 0, time.UTC)
	fim := time.Date(hoje.Year(), 12, 31, 0, 0, 0, 0, time.UTC)
	p := ProjecaoMEI{Faturado: faturado, Teto: teto, Falta: max(teto-faturado, 0), Inicio: inicio,
		MesesRestantes: 13 - int(hoje.Month())}
	if teto > 0 {
		p.Percentual = float64(faturado) / float64(teto)
	}
	p.MediaMaxima = arredondarDivisao(p.Falta, Centavos(p.MesesRestantes))
	dias := int(hoje.Sub(inicio).Hours()/24) + 1
	diasAno := int(fim.Sub(inicio).Hours()/24) + 1
	if dias <= 0 || faturado <= 0 {
		return p
	}
	p.ProjecaoAno = arredondarDivisao(faturado*Centavos(diasAno), Centavos(dias))
	if faturado >= teto {
		d := hoje
		p.DataTeto = &d
		return p
	}
	// dias a partir do início até bater o teto: teto ÷ ritmo, arredondado para cima
	diasTeto := int((int64(teto)*int64(dias) + int64(faturado) - 1) / int64(faturado))
	d := inicio.AddDate(0, 0, diasTeto-1)
	if !d.After(fim) {
		p.DataTeto = &d
	}
	return p
}

// RegrasDASMEI (regras_fiscais: das_mei).
type RegrasDASMEI struct {
	INSS          Centavos `json:"inss_centavos"`
	ISS           Centavos `json:"iss_centavos"`
	ICMS          Centavos `json:"icms_centavos"`
	VencimentoDia int      `json:"vencimento_dia"`
}

// DASMEI do mês pela atividade: serviços (INSS + ISS), comércio ou indústria (INSS +
// ICMS) ou os dois.
func DASMEI(r RegrasDASMEI, atividade string) Centavos {
	switch atividade {
	case atividadeComercio:
		return r.INSS + r.ICMS
	case atividadeAmbos:
		return r.INSS + r.ICMS + r.ISS
	default:
		return r.INSS + r.ISS
	}
}

// LucroMEI no IRPF do titular.
type LucroMEI struct {
	Receita    Centavos `json:"receita_centavos"`
	Despesas   Centavos `json:"despesas_centavos"`
	Lucro      Centavos `json:"lucro_centavos"`
	Presumido  Centavos `json:"presumido_centavos"` // 32 % de serviços + 8 % de comércio
	Isento     Centavos `json:"isento_centavos"`
	Tributavel Centavos `json:"tributavel_centavos"`
}

// CalcularLucroMEI: lucro = receitas − despesas; a parte isenta é o lucro presumido
// (percentuais sobre a receita de cada atividade), limitada ao lucro; o resto é tributável
// na declaração da pessoa física.
func CalcularLucroMEI(servicos, comercio, despesas Centavos, pctServicos, pctComercio string) LucroMEI {
	l := LucroMEI{Receita: servicos + comercio, Despesas: despesas}
	l.Lucro = l.Receita - despesas
	for _, x := range []struct {
		base Centavos
		pct  string
	}{{servicos, pctServicos}, {comercio, pctComercio}} {
		if r, ok := rat(x.pct); ok {
			l.Presumido += arredondarRat(new(big.Rat).Mul(new(big.Rat).SetInt64(int64(x.base)), r))
		}
	}
	l.Isento = max(min(l.Presumido, l.Lucro), 0)
	l.Tributavel = max(l.Lucro-l.Isento, 0)
	return l
}
