package core

import (
	"errors"
	"math"
	"math/big"
	"time"
)

// ComparacaoFinanciamento: parcelar ou pagar à vista, trazendo as parcelas para hoje pelo
// que o dinheiro renderia aplicado.
type ComparacaoFinanciamento struct {
	TotalParcelado        Centavos `json:"total_parcelado_centavos"`
	ValorPresenteParcelas Centavos `json:"valor_presente_parcelas_centavos"`
	JurosImplicitosMes    float64  `json:"juros_implicitos_mes"` // taxa que iguala as parcelas ao preço à vista
	CompensaParcelar      bool     `json:"compensa_parcelar"`    // as parcelas valem menos hoje que o preço à vista
	Diferenca             Centavos `json:"diferenca_centavos"`   // à vista − valor presente (positivo: parcelar ganha)
}

// CompararFinanciamento: n parcelas iguais, a primeira daqui a `primeira` meses (0 = hoje,
// como entrada), contra o preço à vista; rendimentoMes é o que o dinheiro renderia
// aplicado (ex.: CDI líquido ao mês).
func CompararFinanciamento(avista, parcela Centavos, n, primeira int, rendimentoMes float64) (ComparacaoFinanciamento, error) {
	if avista <= 0 || parcela <= 0 || n < 1 || n > 600 || primeira < 0 || primeira > 12 || rendimentoMes <= -1 {
		return ComparacaoFinanciamento{}, errors.New("valores da simulação inválidos")
	}
	vp := func(taxa float64) float64 {
		s := 0.0
		for k := primeira; k < primeira+n; k++ {
			s += float64(parcela) / math.Pow(1+taxa, float64(k))
		}
		return s
	}
	c := ComparacaoFinanciamento{TotalParcelado: parcela * Centavos(n), ValorPresenteParcelas: paraCentavos(vp(rendimentoMes))}
	c.Diferenca = avista - c.ValorPresenteParcelas
	c.CompensaParcelar = c.Diferenca > 0
	// juros implícitos: bisseção em vp(t) = à vista (vp cai com a taxa)
	baixo, alto := -0.99, 10.0
	for range 200 {
		meio := (baixo + alto) / 2
		if vp(meio) > float64(avista) {
			baixo = meio
		} else {
			alto = meio
		}
	}
	c.JurosImplicitosMes = math.Round((baixo+alto)/2*1e8) / 1e8
	return c, nil
}

// Modos de amortização extra.
const (
	ReduzirPrazo   = "prazo"
	ReduzirParcela = "parcela"
)

// SimulacaoQuitacao: o que muda com um pagamento extra numa dívida.
type SimulacaoQuitacao struct {
	SaldoAntes        Centavos `json:"saldo_antes_centavos"`
	SaldoDepois       Centavos `json:"saldo_depois_centavos"`
	ParcelasRestantes int      `json:"parcelas_restantes"`
	NovasParcelas     int      `json:"novas_parcelas"`
	PrestacaoAtual    Centavos `json:"prestacao_atual_centavos"`
	NovaPrestacao     Centavos `json:"nova_prestacao_centavos"`
	JurosRestantes    Centavos `json:"juros_restantes_centavos"`
	NovosJuros        Centavos `json:"novos_juros_centavos"`
	Economia          Centavos `json:"economia_centavos"`
	Quitada           bool     `json:"quitada"`
}

// SimularAmortizacaoExtra: paga `extra` na data (depois das parcelas vencidas até ela) e
// reduz o prazo (mantém a prestação no Price, a amortização no SAC) ou a parcela
// (mantém o número de parcelas).
func SimularAmortizacaoExtra(sistema string, tabela []ParcelaDivida, taxa *big.Float, data time.Time, extra Centavos, modo string) (SimulacaoQuitacao, error) {
	var s SimulacaoQuitacao
	if len(tabela) == 0 || extra <= 0 || (modo != ReduzirPrazo && modo != ReduzirParcela) {
		return s, errors.New("simulação de amortização inválida")
	}
	s.SaldoAntes = tabela[0].SaldoDevedor + tabela[0].Amortizacao
	var restantes []ParcelaDivida
	for _, p := range tabela {
		if p.Vencimento.After(data) {
			restantes = append(restantes, p)
		} else {
			s.SaldoAntes = p.SaldoDevedor
		}
	}
	if len(restantes) == 0 || s.SaldoAntes <= 0 {
		return s, errors.New("a dívida já foi paga")
	}
	s.ParcelasRestantes, s.PrestacaoAtual = len(restantes), restantes[0].Prestacao
	for _, p := range restantes {
		s.JurosRestantes += p.Juros
	}
	if extra >= s.SaldoAntes {
		s.Quitada, s.Economia = true, s.JurosRestantes
		return s, nil
	}
	s.SaldoDepois = s.SaldoAntes - extra

	if modo == ReduzirParcela {
		nova, err := TabelaAmortizacao(sistema, s.SaldoDepois, taxa, len(restantes), restantes[0].Vencimento)
		if err != nil {
			return s, err
		}
		s.NovasParcelas, s.NovaPrestacao = len(nova), nova[0].Prestacao
		for _, p := range nova {
			s.NovosJuros += p.Juros
		}
	} else {
		saldo := s.SaldoDepois
		for saldo > 0 && s.NovasParcelas < 600 {
			juros := arredondar(new(big.Float).SetPrec(precisao).Mul(flt(saldo), taxa))
			amort := restantes[0].Amortizacao // SAC: amortização fixa
			if sistema == SistemaPrice {
				amort = restantes[0].Prestacao - juros
			}
			if amort > saldo {
				amort = saldo
			}
			saldo -= amort
			s.NovosJuros += juros
			s.NovasParcelas++
		}
		s.NovaPrestacao = s.PrestacaoAtual
	}
	s.Economia = s.JurosRestantes - s.NovosJuros
	return s, nil
}
