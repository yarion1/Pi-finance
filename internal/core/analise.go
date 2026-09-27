package core

import (
	"slices"
	"strings"
)

// Análises dos gráficos da fase 7 (Fluxo e Gastos).

// ItemValor: um pedaço com nome e valor (positivo).
type ItemValor struct {
	ID    string   `json:"id"`
	Nome  string   `json:"nome"`
	Valor Centavos `json:"valor_centavos"`
}

// GastoHierarquia: categoria mãe com as filhas.
type GastoHierarquia struct {
	ItemValor
	Cor    *string     `json:"cor"`
	Filhas []ItemValor `json:"filhas"`
}

// NoSankey: Nome é a chave única; Rotulo é o que a tela mostra; Categoria leva ao filtro.
type NoSankey struct {
	Nome      string  `json:"nome"`
	Rotulo    string  `json:"rotulo"`
	Tipo      string  `json:"tipo"` // receita, renda, categoria, subcategoria, sobra, deficit
	Categoria string  `json:"categoria_id,omitempty"`
	Cor       *string `json:"cor,omitempty"` // categorias e subcategorias: a cor da mãe
}

// LigacaoSankey entre dois nós.
type LigacaoSankey struct {
	Origem  string   `json:"origem"`
	Destino string   `json:"destino"`
	Valor   Centavos `json:"valor_centavos"`
}

// MontarSankey: receitas → Renda → categorias → subcategorias. O que sobra vai para
// "Sobrou"; se os gastos passaram das receitas, a diferença entra como "Saiu do saldo".
// O que a mãe tem além das filhas vira "Outros em <mãe>" (o fluxo fecha em cada nó).
func MontarSankey(receitas []ItemValor, gastos []GastoHierarquia) ([]NoSankey, []LigacaoSankey) {
	nos := []NoSankey{}
	ligs := []LigacaoSankey{}
	var totalR, totalG Centavos
	for _, r := range receitas {
		if r.Valor <= 0 {
			continue
		}
		totalR += r.Valor
		nome := "r:" + r.ID + ":" + r.Nome
		nos = append(nos, NoSankey{Nome: nome, Rotulo: r.Nome, Tipo: "receita", Categoria: r.ID})
		ligs = append(ligs, LigacaoSankey{Origem: nome, Destino: "renda", Valor: r.Valor})
	}
	for _, g := range gastos {
		if g.Valor > 0 {
			totalG += g.Valor
		}
	}
	if totalR == 0 && totalG == 0 {
		return nos, ligs
	}
	nos = append(nos, NoSankey{Nome: "renda", Rotulo: "Entradas", Tipo: "renda"})
	if totalG > totalR {
		nos = append(nos, NoSankey{Nome: "deficit", Rotulo: "Saiu do saldo", Tipo: "deficit"})
		ligs = append(ligs, LigacaoSankey{Origem: "deficit", Destino: "renda", Valor: totalG - totalR})
	}
	for _, g := range gastos {
		if g.Valor <= 0 {
			continue
		}
		mae := "c:" + g.ID + ":" + g.Nome
		nos = append(nos, NoSankey{Nome: mae, Rotulo: g.Nome, Tipo: "categoria", Categoria: g.ID, Cor: g.Cor})
		ligs = append(ligs, LigacaoSankey{Origem: "renda", Destino: mae, Valor: g.Valor})
		var dasFilhas Centavos
		for _, f := range g.Filhas {
			if f.Valor <= 0 {
				continue
			}
			dasFilhas += f.Valor
			nome := "s:" + f.ID + ":" + f.Nome
			nos = append(nos, NoSankey{Nome: nome, Rotulo: f.Nome, Tipo: "subcategoria", Categoria: f.ID, Cor: g.Cor})
			ligs = append(ligs, LigacaoSankey{Origem: mae, Destino: nome, Valor: f.Valor})
		}
		if dasFilhas > 0 && g.Valor > dasFilhas {
			nome := "o:" + g.ID
			nos = append(nos, NoSankey{Nome: nome, Rotulo: "Outros em " + g.Nome, Tipo: "subcategoria", Categoria: g.ID, Cor: g.Cor})
			ligs = append(ligs, LigacaoSankey{Origem: mae, Destino: nome, Valor: g.Valor - dasFilhas})
		}
	}
	if totalR > totalG {
		nos = append(nos, NoSankey{Nome: "sobra", Rotulo: "Sobrou", Tipo: "sobra"})
		ligs = append(ligs, LigacaoSankey{Origem: "renda", Destino: "sobra", Valor: totalR - totalG})
	}
	return nos, ligs
}

// Estabelecimento: gastos agrupados pela descrição parecida (ChaveHistorico).
type Estabelecimento struct {
	Nome  string   `json:"nome"`
	Chave string   `json:"chave"`
	Total Centavos `json:"total_centavos"`
	Vezes int      `json:"vezes"`
}

// AgruparEstabelecimentos: soma os gastos (valores negativos) por estabelecimento; o nome
// é a descrição que mais aparece. Os n maiores.
func AgruparEstabelecimentos(descricoes []string, valores []Centavos, n int) []Estabelecimento {
	type acc struct {
		Estabelecimento
		nomes map[string]int
		ordem int
	}
	grupos := map[string]*acc{}
	for i, d := range descricoes {
		if i >= len(valores) || valores[i] >= 0 {
			continue
		}
		chave := ChaveHistorico(d)
		if chave == "" {
			continue
		}
		g := grupos[chave]
		if g == nil {
			g = &acc{Estabelecimento: Estabelecimento{Chave: chave}, nomes: map[string]int{}, ordem: len(grupos)}
			grupos[chave] = g
		}
		g.Total -= valores[i]
		g.Vezes++
		g.nomes[strings.TrimSpace(d)]++
	}
	lista := make([]*acc, 0, len(grupos))
	for _, g := range grupos {
		melhor, vezes := "", 0
		for nome, v := range g.nomes {
			if v > vezes || (v == vezes && nome < melhor) {
				melhor, vezes = nome, v
			}
		}
		g.Nome = melhor
		lista = append(lista, g)
	}
	slices.SortFunc(lista, func(a, b *acc) int {
		if a.Total != b.Total {
			return int(b.Total - a.Total)
		}
		return a.ordem - b.ordem
	})
	out := []Estabelecimento{}
	for _, g := range lista {
		if len(out) == n {
			break
		}
		out = append(out, g.Estabelecimento)
	}
	return out
}
