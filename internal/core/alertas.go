package core

import (
	"slices"
	"strings"
	"time"
)

// Alertas inteligentes das transações novas (SPEC seção 6): gasto fora do padrão da
// categoria, cobrança duplicada, tarifa bancária e juros/IOF. "Assinatura que subiu"
// vem da detecção de recorrências.

// TransacaoAlerta: o mínimo de uma transação de gasto para os alertas.
type TransacaoAlerta struct {
	ID        string
	Conta     string
	Categoria string // vazia quando sem categoria
	Descricao string
	Valor     Centavos // negativo (gasto)
	Data      time.Time
	Parcela   bool // compra parcelada: parcelas iguais não são duplicidade
}

// AlertaDetectado para uma transação nova.
type AlertaDetectado struct {
	Tipo       string // gasto_fora_do_padrao, cobranca_duplicada, tarifa_bancaria, juros_iof
	Transacao  TransacaoAlerta
	Outra      string   // cobranca_duplicada: a outra transação
	Referencia Centavos // gasto_fora_do_padrao: gasto típico da categoria (mediana, positivo)
}

// Limites dos alertas.
const (
	JanelaDuplicada     = 3                // dias entre as duas cobranças
	MinimoDuplicada     = Centavos(50_00)  // abaixo disso, dois cafés iguais não são alerta
	AmostrasPadrao      = 5                // gastos da categoria para ter um padrão
	FatorForaDoPadrao   = 3                // vezes a mediana
	ExcessoForaDoPadrao = Centavos(100_00) // e pelo menos isso acima dela
	MaxAlertasPorRodada = 10               // uma importação grande não vira enxurrada
	DiasRecentesAlertas = 45               // só o que aconteceu há pouco vira alerta
)

var (
	palavrasTarifa = [][]string{{"tarifa"}, {"tar"}, {"anuidade"}, {"cesta", "servicos"}, {"pacote", "servicos"},
		{"manutencao", "conta"}, {"taxa", "saque"}, {"tarifa", "saque"}}
	palavrasJuros = [][]string{{"juros"}, {"iof"}, {"encargos"}, {"encargo"}, {"multa"}, {"mora"}, {"rotativo"}}
)

// temPalavras: todas as palavras aparecem (inteiras) na descrição.
func temPalavras(palavras []string, grupos [][]string) bool {
	for _, g := range grupos {
		if !slices.ContainsFunc(g, func(p string) bool { return !slices.Contains(palavras, p) }) {
			return true
		}
	}
	return false
}

// DetectarAlertas compara as transações novas com o histórico (gastos anteriores, sem as
// novas). Só gastos recentes (até DiasRecentesAlertas antes de hoje) geram alerta; no
// máximo MaxAlertasPorRodada, os de valor maior primeiro.
func DetectarAlertas(novas, historico []TransacaoAlerta, hoje time.Time) []AlertaDetectado {
	limite := hoje.AddDate(0, 0, -DiasRecentesAlertas)
	porCategoria := map[string][]Centavos{}
	for _, h := range historico {
		if h.Categoria != "" && h.Valor < 0 {
			porCategoria[h.Categoria] = append(porCategoria[h.Categoria], -h.Valor)
		}
	}
	medianas := map[string]Centavos{}
	for c, vs := range porCategoria {
		if len(vs) >= AmostrasPadrao {
			medianas[c] = Mediana(vs)
		}
	}

	var out []AlertaDetectado
	todas := append(slices.Clone(historico), novas...)
	for _, t := range novas {
		if t.Valor >= 0 || t.Data.Before(limite) || t.Data.After(hoje) {
			continue
		}
		valor := -t.Valor
		palavras := strings.Fields(ChaveHistorico(t.Descricao))
		switch {
		case temPalavras(palavras, palavrasJuros):
			out = append(out, AlertaDetectado{Tipo: "juros_iof", Transacao: t})
			continue
		case temPalavras(palavras, palavrasTarifa):
			out = append(out, AlertaDetectado{Tipo: "tarifa_bancaria", Transacao: t})
			continue
		}
		if outra, ok := duplicada(t, todas); ok {
			out = append(out, AlertaDetectado{Tipo: "cobranca_duplicada", Transacao: t, Outra: outra})
			continue
		}
		if m, ok := medianas[t.Categoria]; ok && valor >= m*FatorForaDoPadrao && valor-m >= ExcessoForaDoPadrao {
			out = append(out, AlertaDetectado{Tipo: "gasto_fora_do_padrao", Transacao: t, Referencia: m})
		}
	}
	slices.SortStableFunc(out, func(a, b AlertaDetectado) int { return int(a.Transacao.Valor - b.Transacao.Valor) })
	if len(out) > MaxAlertasPorRodada {
		out = out[:MaxAlertasPorRodada]
	}
	return out
}

// duplicada: outra cobrança na mesma conta, mesmo valor e descrição parecida, até
// JanelaDuplicada dias antes (a mais antiga das duas não gera alerta: só a repetida).
func duplicada(t TransacaoAlerta, todas []TransacaoAlerta) (string, bool) {
	if t.Parcela || -t.Valor < MinimoDuplicada {
		return "", false
	}
	chave := ChaveHistorico(t.Descricao)
	if chave == "" {
		return "", false
	}
	for _, o := range todas {
		if o.ID == t.ID || o.Conta != t.Conta || o.Valor != t.Valor || o.Parcela {
			continue
		}
		dias := t.Data.Sub(o.Data).Hours() / 24
		// a outra veio antes (ou no mesmo dia, com id menor, para só uma das duas alertar)
		if dias < 0 || dias > JanelaDuplicada || (dias == 0 && o.ID > t.ID) {
			continue
		}
		if ChaveHistorico(o.Descricao) == chave {
			return o.ID, true
		}
	}
	return "", false
}

// Mediana de valores (a média dos dois do meio quando a quantidade é par).
func Mediana(vs []Centavos) Centavos {
	if len(vs) == 0 {
		return 0
	}
	s := slices.Clone(vs)
	slices.Sort(s)
	n := len(s)
	if n%2 == 1 {
		return s[n/2]
	}
	return (s[n/2-1] + s[n/2]) / 2
}
