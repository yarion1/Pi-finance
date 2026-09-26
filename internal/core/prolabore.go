package core

import "math/big"

// Pró-labore: INSS do sócio (11 % até o teto), IRRF mensal com a redução de 2026
// (Lei 15.270/2025) e o IRRF de 10 % dos lucros acima do limite mensal.

// RegrasINSS do contribuinte individual sobre o pró-labore (regras_fiscais: inss_prolabore).
type RegrasINSS struct {
	Aliquota string   `json:"aliquota"`
	Teto     Centavos `json:"teto_centavos"`
}

// INSSProlabore = alíquota × min(pró-labore, teto), ao centavo.
func INSSProlabore(prolabore Centavos, r RegrasINSS) Centavos {
	a, ok := rat(r.Aliquota)
	if !ok || prolabore <= 0 {
		return 0
	}
	base := prolabore
	if r.Teto > 0 {
		base = min(base, r.Teto)
	}
	return arredondarRat(new(big.Rat).Mul(new(big.Rat).SetInt64(int64(base)), a))
}

// FaixaIRRF da tabela progressiva mensal; Ate 0 = sem limite (última faixa).
type FaixaIRRF struct {
	Ate      Centavos `json:"ate_centavos"`
	Aliquota string   `json:"aliquota"`
	Deduzir  Centavos `json:"deduzir_centavos"`
}

// ReducaoIRRF mensal (Lei 15.270/2025): até Ate, reduz até Maxima (zera o imposto); de
// Ate a AteParcial, reduz Fixo − Coeficiente × rendimentos.
type ReducaoIRRF struct {
	Ate         Centavos `json:"ate_centavos"`
	Maxima      Centavos `json:"maxima_centavos"`
	AteParcial  Centavos `json:"ate_parcial_centavos"`
	Fixo        Centavos `json:"fixo_centavos"`
	Coeficiente string   `json:"coeficiente"`
}

// TabelaIRRF mensal (regras_fiscais: tabela_irrf).
type TabelaIRRF struct {
	Faixas               []FaixaIRRF  `json:"faixas"`
	DescontoSimplificado Centavos     `json:"desconto_simplificado_centavos"`
	Dependente           Centavos     `json:"dependente_centavos"`
	Reducao              *ReducaoIRRF `json:"reducao"`
}

// IRRF do mês sobre rendimentos tributáveis (pró-labore, salário). As deduções legais
// (INSS e dependentes) são trocadas pelo desconto simplificado quando ele é maior. Depois
// da tabela, aplica a redução, que nunca deixa o imposto negativo.
func IRRF(rendimentos, inss Centavos, dependentes int, t TabelaIRRF) Centavos {
	if rendimentos <= 0 {
		return 0
	}
	deducoes := inss + t.Dependente*Centavos(dependentes)
	base := rendimentos - max(deducoes, t.DescontoSimplificado)
	if base <= 0 {
		return 0
	}
	var imposto Centavos
	for _, f := range t.Faixas {
		if f.Ate == 0 || base <= f.Ate {
			a, ok := rat(f.Aliquota)
			if !ok {
				return 0
			}
			v := new(big.Rat).Mul(new(big.Rat).SetInt64(int64(base)), a)
			imposto = max(arredondarRat(v)-f.Deduzir, 0)
			break
		}
	}
	if r := t.Reducao; r != nil && imposto > 0 {
		var reducao Centavos
		switch {
		case rendimentos <= r.Ate:
			reducao = r.Maxima
		case rendimentos <= r.AteParcial:
			if c, ok := rat(r.Coeficiente); ok {
				reducao = r.Fixo - arredondarRat(new(big.Rat).Mul(new(big.Rat).SetInt64(int64(rendimentos)), c))
			}
		}
		imposto -= min(max(reducao, 0), imposto)
	}
	return imposto
}

// LimiteDividendos (regras_fiscais: limite_dividendos_mes).
type LimiteDividendos struct {
	Centavos     Centavos `json:"centavos"`
	AliquotaIRRF string   `json:"aliquota_irrf"`
}

// IRRFDividendos do mês, da mesma empresa para a mesma pessoa: acima do limite, a
// alíquota vale sobre o total pago no mês (não só o excedente). jaPago inclui o que já
// saiu no mês; devolve o IRRF total do mês e quanto cabe ainda sem imposto.
func IRRFDividendos(jaPago, novo Centavos, l LimiteDividendos) (irrf, folga Centavos) {
	total := jaPago + novo
	folga = max(l.Centavos-total, 0)
	if total <= l.Centavos {
		return 0, folga
	}
	a, ok := rat(l.AliquotaIRRF)
	if !ok {
		return 0, folga
	}
	return arredondarRat(new(big.Rat).Mul(new(big.Rat).SetInt64(int64(total)), a)), 0
}
