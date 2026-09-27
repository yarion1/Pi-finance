package core

import (
	"errors"
	"sort"
)

// Despesas divididas da casa: como repartir uma compra e o saldo de "quem deve quem".

// Parte de uma despesa dividida (valor é a parte da pessoa, positivo).
type Parte struct {
	UsuarioID string   `json:"usuario_id"`
	Valor     Centavos `json:"valor_centavos"`
}

// Erros de divisão (mensagens para a pessoa).
var (
	ErrDivisaoVazia = errors.New("escolha ao menos duas pessoas")
	ErrDivisaoSoma  = errors.New("as partes precisam somar o valor da compra")
	ErrDivisaoParte = errors.New("cada parte precisa ser maior que zero")
	ErrDivisaoDupla = errors.New("a mesma pessoa aparece duas vezes")
)

func conferirPessoas(ids []string) error {
	if len(ids) < 2 {
		return ErrDivisaoVazia
	}
	vistos := map[string]bool{}
	for _, id := range ids {
		if vistos[id] {
			return ErrDivisaoDupla
		}
		vistos[id] = true
	}
	return nil
}

// DividirIgual: partes iguais; os centavos que sobram vão para as primeiras pessoas.
func DividirIgual(total Centavos, pessoas []string) ([]Parte, error) {
	if err := conferirPessoas(pessoas); err != nil {
		return nil, err
	}
	if total <= 0 {
		return nil, ErrDivisaoParte
	}
	valores := DividirParcelas(total, len(pessoas))
	out := make([]Parte, len(pessoas))
	for i, p := range pessoas {
		out[i] = Parte{UsuarioID: p, Valor: valores[i]}
	}
	return out, nil
}

// DividirPorPercentual: percentuais em centésimos de ponto (5000 = 50 %), somando 10000.
// Cada parte é arredondada para baixo e os centavos que sobram vão, um a um, para quem
// perdeu mais no arredondamento (a soma fecha sempre com o total).
func DividirPorPercentual(total Centavos, pessoas []string, basis []int64) ([]Parte, error) {
	if err := conferirPessoas(pessoas); err != nil {
		return nil, err
	}
	if len(basis) != len(pessoas) {
		return nil, ErrDivisaoSoma
	}
	var soma int64
	for _, b := range basis {
		if b <= 0 {
			return nil, ErrDivisaoParte
		}
		soma += b
	}
	if soma != 10000 || total <= 0 {
		return nil, ErrDivisaoSoma
	}
	out := make([]Parte, len(pessoas))
	restos := make([]int64, len(pessoas))
	var distribuido Centavos
	for i, p := range pessoas {
		bruto := int64(total) * basis[i]
		out[i] = Parte{UsuarioID: p, Valor: Centavos(bruto / 10000)}
		restos[i] = bruto % 10000
		distribuido += out[i].Valor
	}
	ordem := make([]int, len(pessoas))
	for i := range ordem {
		ordem[i] = i
	}
	sort.SliceStable(ordem, func(a, b int) bool { return restos[ordem[a]] > restos[ordem[b]] })
	for i := 0; distribuido < total; i++ {
		out[ordem[i%len(ordem)]].Valor++
		distribuido++
	}
	for _, p := range out {
		if p.Valor <= 0 {
			return nil, ErrDivisaoParte
		}
	}
	return out, nil
}

// DividirPorValor: a pessoa diz quanto é de cada um; precisa fechar com o total.
func DividirPorValor(total Centavos, partes []Parte) ([]Parte, error) {
	ids := make([]string, len(partes))
	var soma Centavos
	for i, p := range partes {
		if p.Valor <= 0 {
			return nil, ErrDivisaoParte
		}
		ids[i] = p.UsuarioID
		soma += p.Valor
	}
	if err := conferirPessoas(ids); err != nil {
		return nil, err
	}
	if soma != total {
		return nil, ErrDivisaoSoma
	}
	return append([]Parte(nil), partes...), nil
}

// Divida em aberto: devedor deve valor a credor (quem pagou a compra).
type Divida struct {
	Devedor string
	Credor  string
	Valor   Centavos
}

// SaldoCom: quanto a outra pessoa deve a mim (positivo) ou eu a ela (negativo).
type SaldoCom struct {
	UsuarioID string   `json:"usuario_id"`
	Valor     Centavos `json:"valor_centavos"`
}

// QuemDeveQuem: o saldo líquido entre eu e cada pessoa, compensando as dívidas nos dois
// sentidos (acerto por par: cada um acerta direto com o outro). Sem os saldos zerados;
// maiores primeiro.
func QuemDeveQuem(eu string, dividas []Divida) []SaldoCom {
	por := map[string]Centavos{}
	for _, d := range dividas {
		switch {
		case d.Devedor == d.Credor:
		case d.Credor == eu:
			por[d.Devedor] += d.Valor
		case d.Devedor == eu:
			por[d.Credor] -= d.Valor
		}
	}
	out := []SaldoCom{}
	for u, v := range por {
		if v != 0 {
			out = append(out, SaldoCom{UsuarioID: u, Valor: v})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		ai, aj := out[i].Valor, out[j].Valor
		if ai < 0 {
			ai = -ai
		}
		if aj < 0 {
			aj = -aj
		}
		if ai != aj {
			return ai > aj
		}
		return out[i].UsuarioID < out[j].UsuarioID
	})
	return out
}
