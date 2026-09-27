package core

import "testing"

func TestMontarSankey(t *testing.T) {
	gastos := []GastoHierarquia{
		{ItemValor: ItemValor{ID: "ali", Nome: "Alimentação", Valor: 1000}, Filhas: []ItemValor{
			{ID: "mer", Nome: "Mercado", Valor: 600}, {ID: "rest", Nome: "Restaurante", Valor: 300}, {ID: "z", Nome: "Zero", Valor: 0}}},
		{ItemValor: ItemValor{ID: "mor", Nome: "Moradia", Valor: 2000}},
		{ItemValor: ItemValor{ID: "neg", Nome: "Estorno", Valor: -5}},
	}
	nos, ligs := MontarSankey([]ItemValor{{ID: "sal", Nome: "Salário", Valor: 5000}, {ID: "x", Nome: "Nada", Valor: 0}}, gastos)
	soma := map[string]Centavos{}
	for _, l := range ligs {
		soma[l.Origem+">"+l.Destino] = l.Valor
	}
	if soma["r:sal:Salário>renda"] != 5000 || soma["renda>c:ali:Alimentação"] != 1000 || soma["c:ali:Alimentação>o:ali"] != 100 ||
		soma["renda>sobra"] != 2000 || soma["c:ali:Alimentação>s:mer:Mercado"] != 600 || len(ligs) != 7 {
		t.Fatalf("ligações: %+v", ligs)
	}
	if len(nos) != 8 || nos[len(nos)-1].Rotulo != "Sobrou" {
		t.Fatalf("nós: %+v", nos)
	}
	// gasto maior que a receita: a diferença sai do saldo
	_, ligs = MontarSankey([]ItemValor{{ID: "sal", Nome: "Salário", Valor: 1000}}, gastos[1:2])
	if ligs[1].Origem != "deficit" || ligs[1].Valor != 1000 {
		t.Fatalf("déficit: %+v", ligs)
	}
	if n, l := MontarSankey(nil, nil); len(n) != 0 || len(l) != 0 {
		t.Fatal("vazio")
	}
}

func TestAgruparEstabelecimentos(t *testing.T) {
	d := []string{"UBER *TRIP 1", "Uber *trip 2", "UBER *TRIP 3", "IFOOD *X", "SALARIO", "!!!", "PADARIA", "sem valor"}
	v := []Centavos{-1000, -2000, -1500, -9000, 500000, -100, -9000}
	e := AgruparEstabelecimentos(d, v, 2)
	if len(e) != 2 || e[0].Nome != "IFOOD *X" || e[0].Total != 9000 || e[1].Chave != "padaria" {
		t.Fatalf("top 2: %+v", e) // empate em 9000: o que apareceu primeiro
	}
	e = AgruparEstabelecimentos(d, v, 10)
	if len(e) != 3 || e[2].Nome != "UBER *TRIP 1" || e[2].Vezes != 3 || e[2].Total != 4500 {
		t.Fatalf("todos: %+v", e)
	}
}
