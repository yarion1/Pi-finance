package api_test

import (
	"context"
	"encoding/base64"
	"math"
	"net/http"
	"strings"
	"testing"

	"github.com/yarion1/pi-finance/internal/financeiro"
)

const negociacaoB3 = "Data do Negócio;Tipo de Movimentação;Mercado;Prazo/Vencimento;Instituição;Código de Negociação;Quantidade;Preço;Valor\n" +
	"02/01/2026;Compra;Mercado à Vista;-;XP;PETR4;100;30,00;3.000,00\n" +
	"06/01/2026;Compra;Mercado à Vista;-;XP;HGLG11;10;160,00;1.600,00\n" +
	"10/02/2026;Compra;Mercado Fracionário;-;XP;PETR4F;100;40,00;4.000,00\n" +
	"15/03/2026;Venda;Mercado à Vista;-;XP;PETR4;150;50,00;7.500,00\n" +
	"05/04/2026;Compra;Mercado à Vista;-;XP;VALE3;1000;60,00;60.000,00\n" +
	"20/04/2026;Venda;Mercado à Vista;-;XP;VALE3;500;70,00;35.000,00\n" +
	"20/05/2026;Venda;Mercado à Vista;-;XP;HGLG11;10;170,00;1.700,00\n" +
	"21/05/2026;Compra;Opções;-;XP;PETRA300;100;1,00;100,00\n"

const movimentacaoB3 = "Entrada/Saída;Data;Movimentação;Produto;Instituição;Quantidade;Preço unitário;Valor da Operação\n" +
	"Credito;20/03/2026;Dividendo;PETR4 - PETROLEO BRASILEIRO S.A. PETROBRAS;XP;50;1;50,00\n" +
	"Credito;25/03/2026;Desdobro;PETR4 - PETROLEO BRASILEIRO S.A. PETROBRAS;XP;50;-;-\n" +
	"Credito;26/03/2026;Transferência - Liquidação;PETR4 - PETROLEO BRASILEIRO S.A. PETROBRAS;XP;100;38,5;3.850,00\n"

type resultadoB3 struct {
	Formato     string   `json:"formato"`
	Novas       int      `json:"novas"`
	Duplicadas  int      `json:"duplicadas"`
	AtivosNovos []string `json:"ativos_novos"`
	Avisos      []string `json:"avisos"`
}

type carteiraTeste struct {
	Total  int64    `json:"total_centavos"`
	Rent   *float64 `json:"rentabilidade_aa"`
	CDI    *float64 `json:"cdi_aa"`
	Desde  *string  `json:"rentabilidade_desde"`
	Ativos []struct {
		ID          string   `json:"id"`
		Codigo      string   `json:"codigo"`
		Classe      string   `json:"classe"`
		Origem      string   `json:"origem"`
		Quantidade  string   `json:"quantidade"`
		PrecoMedio  string   `json:"preco_medio"`
		Custo       int64    `json:"custo_centavos"`
		Valor       int64    `json:"valor_centavos"`
		Proventos   int64    `json:"proventos_centavos"`
		Encerrado   bool     `json:"encerrado"`
		Vencimento  *string  `json:"vencimento"`
		NaCurva     bool     `json:"na_curva"`
		Rent        *float64 `json:"rentabilidade_aa"`
		Instituicao *string  `json:"instituicao"`
	} `json:"ativos"`
}

func importarB3(t *testing.T, a *cliente, pf, conteudo string, simular bool) resultadoB3 {
	t.Helper()
	var r resultadoB3
	a.exigir("POST", "/api/investimentos/b3", map[string]any{"entidade_id": pf, "simular": simular,
		"conteudo": base64.StdEncoding.EncodeToString([]byte(conteudo))}, http.StatusOK).json(t, &r)
	return r
}

// Extratos da B3 viram operações, eventos e ativos, sem duplicar; o IR do mês sai das vendas.
func TestImportacaoB3EIR(t *testing.T) {
	amb, a, pf, _, _ := prepara(t)

	prev := importarB3(t, a, pf, negociacaoB3, true)
	if prev.Novas != 7 || len(prev.AtivosNovos) != 3 || len(prev.Avisos) != 1 {
		t.Fatalf("prévia: %+v", prev)
	}
	var vazia carteiraTeste
	a.exigir("GET", "/api/investimentos", nil, http.StatusOK).json(t, &vazia)
	if len(vazia.Ativos) != 0 {
		t.Fatal("a prévia não grava nada")
	}
	if r := importarB3(t, a, pf, negociacaoB3, false); r.Novas != 7 || r.Formato != "b3_negociacao" {
		t.Fatalf("negociação: %+v", r)
	}
	if r := importarB3(t, a, pf, movimentacaoB3, false); r.Novas != 2 || len(r.AtivosNovos) != 0 {
		t.Fatalf("movimentação: %+v", r)
	}
	if r := importarB3(t, a, pf, negociacaoB3, false); r.Novas != 0 || r.Duplicadas != 7 {
		t.Fatalf("de novo: %+v", r)
	}
	if r := importarB3(t, a, pf, movimentacaoB3, false); r.Novas != 0 || r.Duplicadas != 2 {
		t.Fatalf("movimentação de novo: %+v", r)
	}
	a.exigir("POST", "/api/investimentos/b3", map[string]any{"entidade_id": pf,
		"conteudo": base64.StdEncoding.EncodeToString([]byte("a;b\n1;2\n"))}, http.StatusBadRequest)

	var c carteiraTeste
	a.exigir("GET", "/api/investimentos", nil, http.StatusOK).json(t, &c)
	ativos := map[string]int{}
	for i, x := range c.Ativos {
		ativos[x.Codigo] = i
	}
	petr := c.Ativos[ativos["PETR4"]]
	// 200 a 35,00 → vende 150 → 50 a 35,00 → desdobro +50 → 100 a 17,50
	if petr.Quantidade != "100" || petr.PrecoMedio != "17.5" || petr.Custo != 175000 || petr.Proventos != 5000 || petr.Origem != "b3" {
		t.Fatalf("PETR4: %+v", petr)
	}
	if v := c.Ativos[ativos["VALE3"]]; v.Quantidade != "500" || v.Custo != 3000000 {
		t.Fatalf("VALE3: %+v", v)
	}
	hglg := c.Ativos[ativos["HGLG11"]]
	if !hglg.Encerrado || hglg.Classe != "acao" {
		t.Fatalf("HGLG11 (sem nome no arquivo, vira unit de ação até a pessoa corrigir): %+v", hglg)
	}
	a.exigir("PATCH", "/api/investimentos/ativos/"+hglg.ID, map[string]any{"classe": "fii", "nome": "CSHG Logística"}, http.StatusNoContent)

	var ir struct {
		Meses []struct {
			Mes        string `json:"mes"`
			Isento     bool   `json:"isento_acoes"`
			Imposto    int64  `json:"imposto_centavos"`
			DARF       int64  `json:"darf_centavos"`
			Vencimento string `json:"vencimento"`
			Codigo     string `json:"codigo_darf"`
			Grupos     []struct {
				Grupo  string `json:"grupo"`
				Ganho  int64  `json:"ganho_centavos"`
				Isento int64  `json:"isento_centavos"`
			} `json:"grupos"`
		} `json:"meses"`
		Vendas []struct{ Ativo string } `json:"vendas"`
		Fonte  string                   `json:"fonte"`
	}
	a.exigir("GET", "/api/investimentos/ir?entidade_id="+pf, nil, http.StatusOK).json(t, &ir)
	if len(ir.Meses) != 3 || len(ir.Vendas) != 3 || ir.Fonte == "" {
		t.Fatalf("apuração: %+v", ir)
	}
	maio, abril, marco := ir.Meses[0], ir.Meses[1], ir.Meses[2]
	if marco.Mes != "2026-03" || !marco.Isento || marco.Imposto != 0 || marco.Grupos[0].Isento != 225000 {
		t.Fatalf("março (vendas até R$ 20 mil: isento): %+v", marco)
	}
	if abril.Imposto != 75000 || abril.DARF != 75000 || abril.Vencimento[:10] != "2026-05-29" || abril.Codigo != "6015" {
		t.Fatalf("abril (R$ 5.000 de ganho a 15%%): %+v", abril)
	}
	if maio.DARF != 2000 || maio.Grupos[0].Grupo != "fii" {
		t.Fatalf("maio (FII a 20%%): %+v", maio)
	}
	// sem entidade na URL: a PF de quem pede
	a.exigir("GET", "/api/investimentos/ir", nil, http.StatusOK).json(t, &ir)
	if len(ir.Meses) != 3 {
		t.Fatalf("IR da PF: %+v", ir)
	}

	// quem não tem acesso à entidade não importa nada nela
	var conv struct{ Link string }
	a.exigir("POST", "/api/convites-conta", nil, http.StatusCreated).json(t, &conv)
	b := amb.novoCliente()
	b.cadastrarCom2FA("Bruno", "bruno@teste.com", conv.Link[strings.LastIndex(conv.Link, "/")+1:])
	b.exigir("POST", "/api/investimentos/b3", map[string]any{"entidade_id": pf,
		"conteudo": base64.StdEncoding.EncodeToString([]byte(negociacaoB3))}, http.StatusNotFound)
	b.exigir("PATCH", "/api/investimentos/ativos/"+hglg.ID, map[string]any{"classe": "acao"}, http.StatusNotFound)
	b.exigir("DELETE", "/api/investimentos/ativos/"+petr.ID, nil, http.StatusNotFound)
}

// Ativo e operações lançados à mão; venda maior que a posição é recusada.
func TestOperacoesManuais(t *testing.T) {
	_, a, pf, _, _ := prepara(t)
	var at struct{ ID string }
	a.exigir("POST", "/api/investimentos/ativos", map[string]any{"entidade_id": pf, "codigo": "Tesouro IPCA+ 2035",
		"classe": "tesouro", "indexador": "ipca", "taxa": "0.065", "vencimento": "2035-05-15"}, http.StatusCreated).json(t, &at)
	a.exigir("POST", "/api/investimentos/ativos", map[string]any{"entidade_id": pf, "codigo": "Tesouro IPCA+ 2035",
		"classe": "tesouro"}, http.StatusBadRequest)
	a.exigir("POST", "/api/investimentos/ativos", map[string]any{"entidade_id": pf, "codigo": "X", "classe": "nada"}, http.StatusBadRequest)

	// editar só a instituição mantém os dados de renda fixa
	a.exigir("PATCH", "/api/investimentos/ativos/"+at.ID, map[string]any{"instituicao": "XP"}, http.StatusNoContent)
	a.exigir("PATCH", "/api/investimentos/ativos/"+at.ID, map[string]any{"indexador": "poupança"}, http.StatusBadRequest)
	var cat carteiraTeste
	a.exigir("GET", "/api/investimentos", nil, http.StatusOK).json(t, &cat)
	if x := cat.Ativos[0]; x.Classe != "tesouro" || x.Vencimento == nil || *x.Vencimento != "2035-05-15" || x.Instituicao == nil {
		t.Fatalf("edição parcial: %+v", x)
	}

	base := "/api/investimentos/ativos/" + at.ID + "/operacoes"
	var op struct{ ID string }
	a.exigir("POST", base, map[string]any{"data": "2026-01-10", "tipo": "compra", "quantidade": "2", "preco": "3000.50",
		"taxas_centavos": 100}, http.StatusCreated).json(t, &op)
	a.exigir("POST", base, map[string]any{"data": "2026-02-10", "tipo": "venda", "quantidade": "5", "preco": "3100"}, http.StatusBadRequest)
	a.exigir("POST", base, map[string]any{"data": "2026-02-10", "tipo": "provento"}, http.StatusBadRequest)
	a.exigir("POST", base, map[string]any{"data": "2026-02-10", "tipo": "venda", "quantidade": "1", "preco": "3100"}, http.StatusCreated)

	var lista []struct {
		ID, Tipo, Quantidade, Preco string
	}
	a.exigir("GET", base, nil, http.StatusOK).json(t, &lista)
	if len(lista) != 2 || lista[0].Tipo != "venda" || lista[1].Preco != "3000.5" {
		t.Fatalf("operações: %+v", lista)
	}
	// apagar a compra deixaria a venda sem posição
	a.exigir("DELETE", base+"/"+op.ID, nil, http.StatusBadRequest)
	a.exigir("DELETE", base+"/"+lista[0].ID, nil, http.StatusNoContent)

	var c carteiraTeste
	a.exigir("GET", "/api/investimentos", nil, http.StatusOK).json(t, &c)
	// sem cotação, o Tesouro IPCA+ rende pela taxa contratada (sem IPCA publicado, só a taxa real)
	if len(c.Ativos) != 1 || c.Ativos[0].Quantidade != "2" || c.Ativos[0].Custo != 600200 || !c.Ativos[0].NaCurva ||
		c.Ativos[0].Valor <= 600200 || c.Ativos[0].Valor > 600200*1065/1000 {
		t.Fatalf("na curva: %+v", c)
	}
	a.exigir("DELETE", "/api/investimentos/ativos/"+at.ID, nil, http.StatusNoContent)
	a.exigir("GET", base, nil, http.StatusNotFound)
}

// Critério de aceite da fase 4 (na tela): um CDB a 100 % do CDI rende o mesmo que o CDI
// no período, até 0,01 p.p. ao ano; a 120 %, rende mais.
func TestRentabilidadeContraCDI(t *testing.T) {
	amb, a, pf, _, _ := prepara(t)
	ctx := context.Background()
	inicio := financeiro.Hoje().AddDate(0, -8, 0)
	if _, err := amb.banco.Dono.Exec(ctx, `insert into indices (serie, data, valor)
		select 'cdi', d, 0.055131 from generate_series($1::date, $2::date, '1 day') d where extract(isodow from d) < 6`,
		inicio.AddDate(0, 0, -10), financeiro.Hoje()); err != nil {
		t.Fatal(err)
	}
	var cdb struct{ ID string }
	a.exigir("POST", "/api/investimentos/ativos", map[string]any{"entidade_id": pf, "codigo": "CDB 100",
		"classe": "renda_fixa", "indexador": "cdi", "taxa": "1"}, http.StatusCreated).json(t, &cdb)
	a.exigir("POST", "/api/investimentos/ativos/"+cdb.ID+"/operacoes", map[string]any{"data": inicio.Format("2006-01-02"),
		"tipo": "compra", "quantidade": "1", "preco": "10000"}, http.StatusCreated)

	var c carteiraTeste
	a.exigir("GET", "/api/investimentos", nil, http.StatusOK).json(t, &c)
	if c.Rent == nil || c.CDI == nil || c.Desde == nil || *c.Desde != inicio.Format("2006-01-02") {
		t.Fatalf("rentabilidade: %+v", c)
	}
	if d := math.Abs(*c.Rent-*c.CDI) * 100; d > 0.01 {
		t.Fatalf("100 %% do CDI: carteira %.6f × CDI %.6f (%.4f p.p.)", *c.Rent, *c.CDI, d)
	}
	if *c.CDI < 0.13 || *c.CDI > 0.16 || c.Ativos[0].Rent == nil {
		t.Fatalf("CDI ao ano: %v", *c.CDI)
	}

	a.exigir("PATCH", "/api/investimentos/ativos/"+cdb.ID, map[string]any{"taxa": "1.2"}, http.StatusNoContent)
	a.exigir("GET", "/api/investimentos", nil, http.StatusOK).json(t, &c)
	if *c.Rent <= *c.CDI {
		t.Fatalf("120 %% do CDI rende mais que o CDI: %v × %v", *c.Rent, *c.CDI)
	}
}

func TestEditarAtivoRecusaCampoDesconhecido(t *testing.T) {
	_, a, pf, _, _ := prepara(t)
	var at struct{ ID string }
	a.exigir("POST", "/api/investimentos/ativos", map[string]any{"entidade_id": pf, "codigo": "X1", "classe": "acao"}, http.StatusCreated).json(t, &at)
	a.exigir("PATCH", "/api/investimentos/ativos/"+at.ID, map[string]any{"entidade_id": "outra"}, http.StatusNoContent)
	a.exigir("PATCH", "/api/investimentos/ativos/"+at.ID, map[string]any{"origem": "pluggy"}, http.StatusBadRequest)
}
