package api_test

import (
	"net/http"
	"strings"
	"testing"
)

// Alertas inteligentes a cada importação: fora do padrão, duplicada, tarifa e IOF.
func TestAlertasInteligentes(t *testing.T) {
	amb, a, pf, nu, _ := prepara(t)
	d := func(n int) string { return strings.ReplaceAll(dia(n), "-", "") }
	var cats []struct{ ID, Nome string }
	a.exigir("GET", "/api/categorias", nil, http.StatusOK).json(t, &cats)
	var mercado string
	for _, c := range cats {
		if c.Nome == "Mercado" {
			mercado = c.ID
		}
	}
	a.exigir("POST", "/api/regras", map[string]any{"entidade_id": pf, "texto": "supermercado", "categoria_id": mercado}, http.StatusCreated)
	// histórico antigo (mais de 45 dias): forma o padrão, sem alertas
	a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": nu, "arquivo": "h.ofx", "conteudo": ofx(
		[4]string{d(-80), "-80.00", "h1", "SUPERMERCADO DIA"},
		[4]string{d(-75), "-90.00", "h2", "SUPERMERCADO DIA"},
		[4]string{d(-70), "-100.00", "h3", "SUPERMERCADO DIA"},
		[4]string{d(-65), "-110.00", "h4", "SUPERMERCADO DIA"},
		[4]string{d(-60), "-120.00", "h5", "SUPERMERCADO DIA"},
		[4]string{d(-58), "-3.00", "h6", "IOF ANTIGO"},
	)}, http.StatusCreated)
	type alerta struct {
		ID    string
		Tipo  string
		Dados map[string]any
	}
	var lista []alerta
	a.exigir("GET", "/api/alertas", nil, http.StatusOK).json(t, &lista)
	if len(lista) != 0 {
		t.Fatalf("histórico antigo não alerta: %+v", lista)
	}

	novas := map[string]any{"conta_id": nu, "arquivo": "n.ofx", "conteudo": ofx(
		[4]string{d(-3), "-450.00", "n1", "SUPERMERCADO EXTRA"},
		[4]string{d(-3), "-150.00", "n2", "SUPERMERCADO DIA"},
		[4]string{d(-4), "-55.90", "n3", "NETFLIX.COM 111"},
		[4]string{d(-2), "-55.90", "n4", "NETFLIX.COM 222"},
		[4]string{d(-2), "-32.90", "n5", "TARIFA PACOTE SERVICOS"},
		[4]string{d(-1), "-4.12", "n6", "IOF COMPRA EXTERIOR"},
	)}
	a.exigir("POST", "/api/importacoes", novas, http.StatusCreated)
	a.exigir("GET", "/api/alertas", nil, http.StatusOK).json(t, &lista)
	tipos := map[string]alerta{}
	for _, x := range lista {
		tipos[x.Tipo] = x
	}
	if len(lista) != 4 || tipos["gasto_fora_do_padrao"].Dados["descricao"] != "SUPERMERCADO EXTRA" ||
		tipos["gasto_fora_do_padrao"].Dados["referencia_centavos"] != float64(10000) ||
		tipos["gasto_fora_do_padrao"].Dados["categoria"] != "Mercado" ||
		tipos["cobranca_duplicada"].Dados["valor_centavos"] != float64(-5590) ||
		tipos["tarifa_bancaria"].Dados["conta"] != "Nubank" || tipos["juros_iof"].Dados["descricao"] != "IOF COMPRA EXTERIOR" {
		t.Fatalf("alertas: %+v", lista)
	}

	// B não dispensa alerta de A
	var casa struct{ ID string }
	a.exigir("POST", "/api/casas", map[string]string{"nome": "Casa"}, http.StatusCreated).json(t, &casa)
	var convite struct{ Link string }
	a.exigir("POST", "/api/casas/"+casa.ID+"/convites", map[string]string{"papel": "membro"}, http.StatusCreated).json(t, &convite)
	b := amb.novoCliente()
	b.cadastrarCom2FA("Bruno", "bruno@teste.com", convite.Link[strings.LastIndex(convite.Link, "/")+1:])
	b.exigir("POST", "/api/alertas/"+tipos["juros_iof"].ID+"/lido", nil, http.StatusNotFound)

	// dispensar tira da lista (e não volta ao importar de novo); ?todos=1 mostra
	a.exigir("POST", "/api/alertas/"+tipos["juros_iof"].ID+"/lido", nil, http.StatusNoContent)
	a.exigir("POST", "/api/importacoes", novas, http.StatusCreated)
	a.exigir("GET", "/api/alertas", nil, http.StatusOK).json(t, &lista)
	if len(lista) != 3 {
		t.Fatalf("depois de dispensar: %+v", lista)
	}
	a.exigir("GET", "/api/alertas?todos=1", nil, http.StatusOK).json(t, &lista)
	if len(lista) != 4 {
		t.Fatalf("todos: %+v", lista)
	}
	a.exigir("POST", "/api/alertas/lidos", nil, http.StatusNoContent)
	a.exigir("GET", "/api/alertas", nil, http.StatusOK).json(t, &lista)
	if len(lista) != 0 {
		t.Fatalf("todos lidos: %+v", lista)
	}
}
