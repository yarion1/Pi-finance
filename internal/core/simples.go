package core

import (
	"errors"
	"math/big"
	"sort"
)

// Simples Nacional (SPEC §4): alíquota efetiva pela RBT12, partilha entre os tributos,
// Fator R e exportação de serviços. As tabelas vêm de regras_fiscais, com vigência.

// Anexos que o painel calcula.
const (
	AnexoIII = "III"
	AnexoV   = "V"
)

// Tributos do DAS, na ordem da partilha.
var TributosDAS = []string{"irpj", "csll", "cofins", "pis", "cpp", "iss"}

// Tributos que saem do DAS na receita de exportação de serviços (PIS, COFINS e ISS).
var tributosExportacao = map[string]bool{"cofins": true, "pis": true, "iss": true}

// FaixaSimples de um anexo (regras_fiscais: faixas_anexo_iii, faixas_anexo_v).
type FaixaSimples struct {
	Ate      Centavos `json:"ate_centavos"`
	Aliquota string   `json:"aliquota"`
	Deduzir  Centavos `json:"deduzir_centavos"`
}

// ErrAcimaDoSimples: RBT12 maior que a última faixa (teto do Simples).
var ErrAcimaDoSimples = errors.New("receita dos últimos 12 meses acima do teto do Simples")

// ErrTabela: faixas ou partilha inválidas.
var ErrTabela = errors.New("tabela do Simples inválida")

func rat(s string) (*big.Rat, bool) {
	if s == "" {
		return new(big.Rat), true
	}
	return new(big.Rat).SetString(s)
}

// arredondarRat ao centavo (meio para longe do zero).
func arredondarRat(r *big.Rat) Centavos {
	return Centavos(dividirArredondando(new(big.Int).Set(r.Num()), r.Denom()).Int64())
}

// FaixaDaRBT12: índice (0 a 5) da faixa em que a RBT12 cai.
func FaixaDaRBT12(rbt12 Centavos, faixas []FaixaSimples) (int, error) {
	for i, f := range faixas {
		if rbt12 <= f.Ate {
			return i, nil
		}
	}
	return 0, ErrAcimaDoSimples
}

// aliquotaEfetiva = (RBT12 × nominal − deduzir) ÷ RBT12. Sem RBT12 (primeiro mês sem
// receita anterior), vale a nominal da primeira faixa.
func aliquotaEfetiva(rbt12 Centavos, faixas []FaixaSimples) (*big.Rat, int, error) {
	if len(faixas) == 0 {
		return nil, 0, ErrTabela
	}
	i, err := FaixaDaRBT12(rbt12, faixas)
	if err != nil {
		return nil, 0, err
	}
	nom, ok := rat(faixas[i].Aliquota)
	if !ok {
		return nil, 0, ErrTabela
	}
	if rbt12 <= 0 {
		return nom, i, nil
	}
	r := new(big.Rat).Mul(new(big.Rat).SetInt64(int64(rbt12)), nom)
	r.Sub(r, new(big.Rat).SetInt64(int64(faixas[i].Deduzir)))
	r.Quo(r, new(big.Rat).SetInt64(int64(rbt12)))
	return r, i, nil
}

// TributoDAS: parcela de um tributo no DAS do mês.
type TributoDAS struct {
	Tributo  string   `json:"tributo"`
	Aliquota string   `json:"aliquota"` // parte da alíquota efetiva que é deste tributo
	Valor    Centavos `json:"valor_centavos"`
}

// ResultadoDAS do mês.
type ResultadoDAS struct {
	Anexo           string       `json:"anexo"`
	Faixa           int          `json:"faixa"` // 1 a 6
	RBT12           Centavos     `json:"rbt12_centavos"`
	Receita         Centavos     `json:"receita_centavos"`
	Exportacao      Centavos     `json:"exportacao_centavos"`
	AliquotaNominal string       `json:"aliquota_nominal"`
	AliquotaEfetiva string       `json:"aliquota_efetiva"` // 8 casas
	DAS             Centavos     `json:"das_centavos"`
	SemExportacao   Centavos     `json:"das_sem_exportacao_centavos"` // se a exportação pagasse tudo
	Tributos        []TributoDAS `json:"tributos"`
}

// ParametrosDAS de um mês.
type ParametrosDAS struct {
	Anexo      string
	Faixas     []FaixaSimples
	Partilha   []map[string]string // uma por faixa: tributo → fração da alíquota ("0.335")
	ISSMaximo  string              // teto do ISS efetivo ("0.05"); a sobra vai aos federais
	RBT12      Centavos
	Receita    Centavos // receita do mês, incluindo a de exportação
	Exportacao Centavos // parte da receita que é exportação de serviços
}

// CalcularDAS: DAS do mês = receita × alíquota efetiva, repartido entre os tributos pela
// partilha da faixa. O ISS efetivo fica limitado ao ISSMaximo e a diferença vai para os
// tributos federais na proporção deles. Na receita de exportação, PIS, COFINS e ISS não
// entram. O total é arredondado ao centavo e a partilha soma exatamente o total (maior
// resto).
func CalcularDAS(p ParametrosDAS) (ResultadoDAS, error) {
	res := ResultadoDAS{Anexo: p.Anexo, RBT12: p.RBT12, Receita: p.Receita, Exportacao: p.Exportacao}
	if p.Exportacao < 0 || p.Exportacao > p.Receita || p.Receita < 0 {
		return res, ErrTabela
	}
	efetiva, i, err := aliquotaEfetiva(p.RBT12, p.Faixas)
	if err != nil {
		return res, err
	}
	if i >= len(p.Partilha) {
		return res, ErrTabela
	}
	res.Faixa, res.AliquotaNominal = i+1, p.Faixas[i].Aliquota
	res.AliquotaEfetiva = efetiva.FloatString(8)

	// partilha da faixa, com o teto do ISS
	partes := map[string]*big.Rat{}
	soma := new(big.Rat)
	for _, t := range TributosDAS {
		v, ok := rat(p.Partilha[i][t])
		if !ok {
			return res, ErrTabela
		}
		partes[t] = new(big.Rat).Mul(efetiva, v)
		soma.Add(soma, v)
	}
	if soma.Cmp(big.NewRat(1, 1)) != 0 {
		return res, ErrTabela
	}
	if issMax, ok := rat(p.ISSMaximo); ok && p.ISSMaximo != "" && partes["iss"].Cmp(issMax) > 0 {
		sobra := new(big.Rat).Sub(partes["iss"], issMax)
		partes["iss"] = issMax
		federais := new(big.Rat).Sub(efetiva, new(big.Rat).Add(issMax, sobra))
		for _, t := range TributosDAS {
			if t == "iss" || federais.Sign() == 0 {
				continue
			}
			extra := new(big.Rat).Mul(sobra, new(big.Rat).Quo(partes[t], federais))
			partes[t] = new(big.Rat).Add(partes[t], extra)
		}
	}

	receita := new(big.Rat).SetInt64(int64(p.Receita))
	semExp := new(big.Rat).SetInt64(int64(p.Receita - p.Exportacao))
	exatos := make([]*big.Rat, len(TributosDAS))
	total, cheio := new(big.Rat), new(big.Rat)
	for k, t := range TributosDAS {
		base := receita
		if tributosExportacao[t] {
			base = semExp
		}
		exatos[k] = new(big.Rat).Mul(base, partes[t])
		total.Add(total, exatos[k])
		cheio.Add(cheio, new(big.Rat).Mul(receita, partes[t]))
	}
	res.DAS, res.SemExportacao = arredondarRat(total), arredondarRat(cheio)
	valores := repartirMaiorResto(exatos, res.DAS)
	for k, t := range TributosDAS {
		res.Tributos = append(res.Tributos, TributoDAS{Tributo: t, Aliquota: partes[t].FloatString(8), Valor: valores[k]})
	}
	return res, nil
}

// repartirMaiorResto arredonda cada parte para baixo e dá os centavos que faltam para
// fechar o total às maiores sobras (empate: a ordem da lista).
func repartirMaiorResto(exatos []*big.Rat, total Centavos) []Centavos {
	out := make([]Centavos, len(exatos))
	type sobra struct {
		i int
		r *big.Rat
	}
	var sobras []sobra
	var soma Centavos
	for i, e := range exatos {
		piso := new(big.Int).Quo(e.Num(), e.Denom()) // partes nunca negativas
		out[i] = Centavos(piso.Int64())
		soma += out[i]
		sobras = append(sobras, sobra{i, new(big.Rat).Sub(e, new(big.Rat).SetInt(piso))})
	}
	sort.SliceStable(sobras, func(a, b int) bool { return sobras[a].r.Cmp(sobras[b].r) > 0 })
	for k := 0; soma < total && k < len(sobras); k++ {
		out[sobras[k].i]++
		soma++
	}
	return out
}

// FatorR = folha dos últimos 12 meses (com pró-labore) ÷ RBT12, com 8 casas. Sem
// receita, 0.
func FatorR(folha12, rbt12 Centavos) string {
	if rbt12 <= 0 {
		return "0.00000000"
	}
	return big.NewRat(int64(folha12), int64(rbt12)).FloatString(8)
}

// AnexoPeloFatorR: Anexo III com Fator R de pelo menos o mínimo (0,28); senão, Anexo V.
// Sem receita nos 12 meses, vale a folha: com folha, III.
func AnexoPeloFatorR(folha12, rbt12 Centavos, minimo string) string {
	m, ok := rat(minimo)
	if !ok {
		return AnexoV
	}
	if rbt12 <= 0 {
		if folha12 > 0 {
			return AnexoIII
		}
		return AnexoV
	}
	if big.NewRat(int64(folha12), int64(rbt12)).Cmp(m) >= 0 {
		return AnexoIII
	}
	return AnexoV
}

// RBT12 do mês: soma dos 12 meses anteriores. No início de atividade (LC 123, art. 18,
// §§ 2º e 3º): com menos de 12 meses, a média dos meses anteriores × 12; no primeiro
// mês, a receita do próprio mês × 12. anteriores vem do mais antigo ao mais recente.
func RBT12(anteriores []Centavos, receitaMes Centavos) Centavos {
	if len(anteriores) > 12 {
		anteriores = anteriores[len(anteriores)-12:]
	}
	var soma Centavos
	for _, v := range anteriores {
		soma += v
	}
	switch {
	case len(anteriores) == 12:
		return soma
	case len(anteriores) == 0:
		return receitaMes * 12
	default:
		return arredondarDivisao(soma*12, Centavos(len(anteriores)))
	}
}

// ProlaboreParaFatorR: menor pró-labore mensal (ao centavo, para cima) que leva a folha
// de 12 meses a mínimo × RBT12, descontada a folha que já existe, e nunca abaixo do
// salário mínimo.
func ProlaboreParaFatorR(rbt12, outrasFolhas12, salarioMinimo Centavos, minimo string) Centavos {
	m, ok := rat(minimo)
	if !ok || rbt12 <= 0 {
		return salarioMinimo
	}
	alvo := new(big.Rat).Mul(new(big.Rat).SetInt64(int64(rbt12)), m)
	alvo.Sub(alvo, new(big.Rat).SetInt64(int64(outrasFolhas12)))
	if alvo.Sign() <= 0 {
		return salarioMinimo
	}
	alvo.Quo(alvo, big.NewRat(12, 1))
	n, d := alvo.Num(), alvo.Denom()
	q, r := new(big.Int).QuoRem(n, d, new(big.Int))
	if r.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	return max(Centavos(q.Int64()), salarioMinimo)
}
