package api_test

import (
	"net/http"
	"testing"
)

func TestAnaliseGastosEFluxo(t *testing.T) {
	_, a, pf, nu, inter := prepara(t)
	var cats []struct {
		ID, Nome string
		PaiID    *string `json:"pai_id"`
	}
	a.exigir("GET", "/api/categorias", nil, http.StatusOK).json(t, &cats)
	id := map[string]string{}
	for _, c := range cats {
		id[c.Nome] = c.ID
	}
	a.exigir("POST", "/api/regras", map[string]any{"entidade_id": pf, "texto": "supermercado", "categoria_id": id["Mercado"]}, http.StatusCreated)
	a.exigir("POST", "/api/regras", map[string]any{"entidade_id": pf, "texto": "restaurante", "categoria_id": id["Restaurante"]}, http.StatusCreated)
	a.exigir("POST", "/api/regras", map[string]any{"entidade_id": pf, "texto": "salario", "categoria_id": id["Salário"]}, http.StatusCreated)
	importarOFX(t, a, nu, ofx(
		[4]string{"20260901", "5000.00", "s1", "SALARIO ACME"},
		[4]string{"20260902", "-300.00", "m1", "SUPERMERCADO DIA 1"},
		[4]string{"20260910", "-200.00", "m2", "SUPERMERCADO DIA 2"},
		[4]string{"20260911", "-100.00", "r1", "RESTAURANTE BOM"},
		[4]string{"20260912", "-50.00", "x1", "LOJA SEM CATEGORIA"},
		[4]string{"20260815", "-400.00", "m0", "SUPERMERCADO DIA 0"},
		[4]string{"20260903", "-1000.00", "t1", "Pix enviado Ana Inter"},
	))
	importarOFX(t, a, inter, ofx([4]string{"20260903", "1000.00", "t2", "Pix recebido Ana Nubank"}))

	var g struct {
		Meses  []string
		Series []struct {
			Nome    string
			Valores []int64 `json:"valores_centavos"`
		}
		Treemap []struct {
			Nome   string
			Valor  int64 `json:"valor_centavos"`
			Filhas []struct {
				Nome  string
				Valor int64 `json:"valor_centavos"`
			}
		}
		Dias             []struct{ Data string }
		Estabelecimentos []struct {
			Nome  string
			Total int64 `json:"total_centavos"`
			Vezes int
		}
	}
	a.exigir("GET", "/api/analise/gastos?mes=2026-09&meses=2", nil, http.StatusOK).json(t, &g)
	if len(g.Meses) != 2 || g.Meses[1] != "2026-09" || len(g.Treemap) != 2 || g.Treemap[0].Nome != "Alimentação" ||
		g.Treemap[0].Valor != 60000 || len(g.Treemap[0].Filhas) != 2 || g.Treemap[0].Filhas[0].Nome != "Mercado" {
		t.Fatalf("treemap: %+v", g)
	}
	if g.Series[0].Nome != "Alimentação" || g.Series[0].Valores[0] != 40000 || g.Series[0].Valores[1] != 60000 {
		t.Fatalf("séries: %+v", g.Series)
	}
	// a transferência não é gasto; o supermercado agrupa as duas compras
	if len(g.Dias) != 5 || g.Estabelecimentos[0].Nome == "" || g.Estabelecimentos[0].Total != 50000 || g.Estabelecimentos[0].Vezes != 2 {
		t.Fatalf("dias e estabelecimentos: %+v %+v", g.Dias, g.Estabelecimentos)
	}

	var f struct {
		Nos      []struct{ Nome, Rotulo, Tipo string }
		Ligacoes []struct {
			Origem, Destino string
			Valor           int64 `json:"valor_centavos"`
		}
		Cascata struct {
			Inicial  int64 `json:"saldo_inicial_centavos"`
			Entradas int64 `json:"entradas_centavos"`
			Saidas   int64 `json:"saidas_centavos"`
			Final    int64 `json:"saldo_final_centavos"`
		}
	}
	a.exigir("GET", "/api/analise/fluxo?mes=2026-09", nil, http.StatusOK).json(t, &f)
	var sobra int64
	for _, l := range f.Ligacoes {
		if l.Destino == "sobra" {
			sobra = l.Valor
		}
	}
	if sobra != 500000-65000 {
		t.Fatalf("sankey: %+v", f)
	}
	// Nubank começa com R$ 1.000 e perde R$ 400 em agosto; setembro: +5.000 e −1.650 (com o Pix) e +1.000 no Inter
	c := f.Cascata
	if c.Inicial != 60000 || c.Entradas != 600000 || c.Saidas != 165000 || c.Final != c.Inicial+c.Entradas-c.Saidas {
		t.Fatalf("cascata: %+v", c)
	}
	a.exigir("GET", "/api/analise/gastos?meses=0", nil, http.StatusBadRequest)
	a.exigir("GET", "/api/analise/fluxo?mes=setembro", nil, http.StatusBadRequest)
}
