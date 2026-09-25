package core

import (
	"errors"
	"math/big"
	"strings"
)

// Dec8 é um número com 8 casas decimais guardado como inteiro (o numeric(24,8) do
// banco): quantidades de ativos e cotações. 1 cota = 100_000_000. Nunca float.
type Dec8 int64

// Um é 1 em Dec8.
const Um Dec8 = 100_000_000

// ErrDec8 indica texto que não é um número com até 8 casas decimais.
var ErrDec8 = errors.New("número inválido (use ponto ou vírgula, até 8 casas)")

// ParseDec8 lê "10", "10.5", "0,00012345" ou "-3.2" (sem separador de milhar).
func ParseDec8(s string) (Dec8, error) {
	s = strings.TrimSpace(strings.Replace(s, ",", ".", 1))
	negativo := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(strings.TrimPrefix(s, "-"), "+")
	inteiro, fracao, _ := strings.Cut(s, ".")
	if inteiro == "" {
		inteiro = "0"
	}
	if !soDigitos(inteiro) || (fracao != "" && !soDigitos(fracao)) || len(fracao) > 8 || len(inteiro) > 10 {
		return 0, ErrDec8
	}
	fracao += strings.Repeat("0", 8-len(fracao))
	var v int64
	for _, c := range inteiro + fracao {
		v = v*10 + int64(c-'0')
	}
	if negativo {
		v = -v
	}
	return Dec8(v), nil
}

// String no formato do banco ("10.5", "0.00012345").
func (d Dec8) String() string {
	sinal := ""
	v := int64(d)
	if v < 0 {
		sinal, v = "-", -v
	}
	inteiro, fracao := v/int64(Um), v%int64(Um)
	s := sinal + itoa64(inteiro)
	if fracao == 0 {
		return s
	}
	f := strings.TrimRight(strings.Repeat("0", 8-len(itoa64(fracao)))+itoa64(fracao), "0")
	return s + "." + f
}

func itoa64(v int64) string { return big.NewInt(v).String() }

// dividirArredondando divide a por b arredondando meio para longe do zero.
func dividirArredondando(a, b *big.Int) *big.Int {
	q, r := new(big.Int).QuoRem(a, b, new(big.Int))
	dobro := new(big.Int).Abs(new(big.Int).Lsh(r, 1))
	if dobro.Cmp(new(big.Int).Abs(b)) >= 0 {
		if (a.Sign() < 0) != (b.Sign() < 0) {
			q.Sub(q, big.NewInt(1))
		} else {
			q.Add(q, big.NewInt(1))
		}
	}
	return q
}

// Valor: quantidade × preço, em centavos (arredonda meio para longe do zero).
func Valor(quantidade, preco Dec8) Centavos {
	p := new(big.Int).Mul(big.NewInt(int64(quantidade)), big.NewInt(int64(preco)))
	// q·p tem 16 casas; centavos têm 2: divide por 10^14
	return Centavos(dividirArredondando(p, new(big.Int).Exp(big.NewInt(10), big.NewInt(14), nil)).Int64())
}

// PrecoMedio: custo ÷ quantidade, com 8 casas.
func PrecoMedio(custo Centavos, quantidade Dec8) Dec8 {
	if quantidade == 0 {
		return 0
	}
	// custo (2 casas) × 10^14 ÷ quantidade (8 casas) = preço com 8 casas
	n := new(big.Int).Mul(big.NewInt(int64(custo)), new(big.Int).Exp(big.NewInt(10), big.NewInt(14), nil))
	return Dec8(dividirArredondando(n, big.NewInt(int64(quantidade))).Int64())
}

// Proporcao: valor × (parte ÷ todo), em centavos. Usada para tirar do custo a parte vendida.
func Proporcao(valor Centavos, parte, todo Dec8) Centavos {
	if todo == 0 {
		return 0
	}
	n := new(big.Int).Mul(big.NewInt(int64(valor)), big.NewInt(int64(parte)))
	return Centavos(dividirArredondando(n, big.NewInt(int64(todo))).Int64())
}

// MultDec8: a × b com 8 casas (ex.: quantidade × fator de desdobramento).
func MultDec8(a, b Dec8) Dec8 {
	n := new(big.Int).Mul(big.NewInt(int64(a)), big.NewInt(int64(b)))
	return Dec8(dividirArredondando(n, big.NewInt(int64(Um))).Int64())
}
