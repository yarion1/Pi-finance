package core

import (
	"errors"
	"math/big"
	"strings"
	"time"
)

// Sistemas de amortização de financiamentos e empréstimos.
const (
	SistemaPrice = "price" // prestação fixa
	SistemaSAC   = "sac"   // amortização fixa, prestação cai
)

// ParcelaDivida é uma linha da tabela de amortização.
type ParcelaDivida struct {
	Numero       int       `json:"numero"`
	Vencimento   time.Time `json:"vencimento"`
	Prestacao    Centavos  `json:"prestacao_centavos"`
	Juros        Centavos  `json:"juros_centavos"`
	Amortizacao  Centavos  `json:"amortizacao_centavos"`
	SaldoDevedor Centavos  `json:"saldo_devedor_centavos"`
}

// ErrTaxa indica taxa mensal inválida.
var ErrTaxa = errors.New("taxa mensal inválida")

const precisao = 256

// LerTaxa lê a taxa ao mês como fração decimal ("0.0099" = 0,99 % a.m.).
func LerTaxa(s string) (*big.Float, error) {
	f, ok := new(big.Float).SetPrec(precisao).SetString(strings.ReplaceAll(strings.TrimSpace(s), ",", "."))
	if !ok || f.Sign() < 0 || f.Cmp(big.NewFloat(1)) >= 0 {
		return nil, ErrTaxa
	}
	return f, nil
}

func flt(c Centavos) *big.Float { return new(big.Float).SetPrec(precisao).SetInt64(int64(c)) }

// arredondar para centavos, metade para longe do zero.
func arredondar(f *big.Float) Centavos {
	meio := big.NewFloat(0.5)
	g := new(big.Float).SetPrec(precisao)
	if f.Sign() < 0 {
		g.Sub(f, meio)
	} else {
		g.Add(f, meio)
	}
	i, _ := g.Int(nil) // trunca em direção ao zero
	return Centavos(i.Int64())
}

// TabelaAmortizacao monta o cronograma (principal em centavos, taxa ao mês, prazo
// em meses, vencimento da primeira parcela). Os valores são arredondados a cada
// parcela e a última zera o saldo.
func TabelaAmortizacao(sistema string, principal Centavos, taxa *big.Float, prazo int, primeiro time.Time) ([]ParcelaDivida, error) {
	if principal <= 0 || prazo < 1 || prazo > 600 {
		return nil, errors.New("principal e prazo precisam ser positivos")
	}
	if sistema != SistemaPrice && sistema != SistemaSAC {
		return nil, errors.New("sistema de amortização: price ou sac")
	}
	tabela := make([]ParcelaDivida, 0, prazo)
	saldo := principal

	var prestacaoPrice Centavos
	if sistema == SistemaPrice {
		if taxa.Sign() == 0 {
			prestacaoPrice = arredondar(new(big.Float).Quo(flt(principal), flt(Centavos(prazo))))
		} else {
			// PMT = P · i / (1 − (1+i)^−n)
			umMais := new(big.Float).SetPrec(precisao).Add(big.NewFloat(1), taxa)
			pot := new(big.Float).SetPrec(precisao).SetInt64(1)
			for range prazo {
				pot.Mul(pot, umMais)
			}
			inv := new(big.Float).SetPrec(precisao).Quo(big.NewFloat(1), pot)
			den := new(big.Float).SetPrec(precisao).Sub(big.NewFloat(1), inv)
			num := new(big.Float).SetPrec(precisao).Mul(flt(principal), taxa)
			prestacaoPrice = arredondar(new(big.Float).SetPrec(precisao).Quo(num, den))
		}
	}
	amortSAC := arredondar(new(big.Float).SetPrec(precisao).Quo(flt(principal), flt(Centavos(prazo))))

	for n := 1; n <= prazo; n++ {
		juros := arredondar(new(big.Float).SetPrec(precisao).Mul(flt(saldo), taxa))
		var amort Centavos
		if sistema == SistemaPrice {
			amort = prestacaoPrice - juros
		} else {
			amort = amortSAC
		}
		if n == prazo || amort > saldo {
			amort = saldo
		}
		saldo -= amort
		tabela = append(tabela, ParcelaDivida{
			Numero: n, Vencimento: SomarMeses(primeiro, n-1),
			Prestacao: amort + juros, Juros: juros, Amortizacao: amort, SaldoDevedor: saldo,
		})
	}
	return tabela, nil
}

// SaldoDevedorEm é o saldo depois da última parcela que venceu até a data.
func SaldoDevedorEm(tabela []ParcelaDivida, principal Centavos, data time.Time) Centavos {
	saldo := principal
	for _, p := range tabela {
		if p.Vencimento.After(truncarDia(data)) {
			break
		}
		saldo = p.SaldoDevedor
	}
	return saldo
}
