package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/financeiro"
	"github.com/yarion1/pi-finance/internal/worker"
)

func TestRelatoriosDoMesEDaSemana(t *testing.T) {
	amb, a, pf, nu, _ := prepara(t)
	agora := financeiro.Agora()
	mes := core.MesDoRelatorio(financeiro.Hoje())
	semana := core.SemanaDoResumo(agora)
	em := func(base time.Time, meses, dias int) string {
		return base.AddDate(0, meses, dias).Format("20060102")
	}
	var cats []struct{ ID, Nome string }
	a.exigir("GET", "/api/categorias", nil, http.StatusOK).json(t, &cats)
	for _, c := range cats {
		if c.Nome == "Mercado" {
			a.exigir("POST", "/api/regras", map[string]any{"entidade_id": pf, "texto": "supermercado", "categoria_id": c.ID}, http.StatusCreated)
		}
	}
	importarOFX(t, a, nu, ofx(
		[4]string{em(mes, 0, 1), "5000.00", "r1", "SALARIO"},
		[4]string{em(mes, 0, 4), "-800.00", "m0", "SUPERMERCADO"},
		[4]string{em(mes, 0, 9), "-55.90", "n0", "NETFLIX"},
		[4]string{em(mes, -1, 4), "-300.00", "m1", "SUPERMERCADO"},
		[4]string{em(mes, -2, 4), "-300.00", "m2", "SUPERMERCADO"},
		[4]string{em(mes, -3, 4), "-300.00", "m3", "SUPERMERCADO"},
		[4]string{em(semana, 0, 1), "-20.00", "p1", "PADARIA"},
	))
	ligarIA(t, a, 5_000_000)
	amb.claude.responde = func(p map[string]any) string {
		return respostaClaude("end_turn", `{"type":"text","text":"Resumo de teste do mês."}`)
	}
	if err := worker.GerarRelatorios(context.Background(), amb.banco.App, amb.srv.IA, agora); err != nil {
		t.Fatal(err)
	}
	var lista []struct{ ID, Tipo, Periodo string }
	a.exigir("GET", "/api/relatorios", nil, http.StatusOK).json(t, &lista)
	if len(lista) != 2 {
		t.Fatalf("mês e semana: %+v", lista)
	}
	var mensal, semanal string
	for _, r := range lista {
		switch {
		case r.Tipo == "mes" && r.Periodo == mes.Format("2006-01"):
			mensal = r.ID
		case r.Tipo == "semana" && r.Periodo == semana.Format("2006-01-02"):
			semanal = r.ID
		}
	}
	if mensal == "" || semanal == "" {
		t.Fatalf("períodos: %+v", lista)
	}

	var rel struct {
		Texto *string
		Dados struct {
			Totais struct {
				Receitas int64 `json:"receitas_centavos"`
				Gastos   int64 `json:"gastos_centavos"`
			} `json:"totais"`
			Categorias []struct {
				Nome   string
				Total  int64 `json:"total_centavos"`
				Media3 int64 `json:"media_3_meses_centavos"`
			} `json:"categorias"`
			Acumulado  []json.RawMessage `json:"acumulado"`
			Patrimonio []json.RawMessage `json:"patrimonio"`
			Acoes      []string          `json:"acoes"`
		}
	}
	a.exigir("GET", "/api/relatorios/"+mensal, nil, http.StatusOK).json(t, &rel)
	d := rel.Dados
	if d.Totais.Receitas != 500000 || d.Totais.Gastos < 85590 || len(d.Acumulado) != core.DiasNoMes(mes.Year(), mes.Month()) ||
		len(d.Patrimonio) == 0 || len(d.Acoes) == 0 || len(d.Acoes) > 3 {
		t.Fatalf("relatório: %+v", d)
	}
	if d.Categorias[0].Nome != "Alimentação" || d.Categorias[0].Total != 80000 || d.Categorias[0].Media3 != 30000 ||
		!strings.Contains(strings.ReplaceAll(d.Acoes[0], " ", " "), "Alimentação subiu R$ 500,00") {
		t.Fatalf("categorias e ações: %+v %q", d.Categorias, d.Acoes)
	}
	if rel.Texto == nil || *rel.Texto != "Resumo de teste do mês." {
		t.Fatalf("texto da IA: %v", rel.Texto)
	}
	// só números e categorias vão para a IA, nunca descrições de transação
	pedido, _ := json.Marshal(amb.claude.pedidos[len(amb.claude.pedidos)-1]["messages"])
	if strings.Contains(string(pedido), "NETFLIX") || strings.Contains(string(pedido), "SUPERMERCADO") {
		t.Fatalf("descrição foi para a IA: %s", pedido)
	}

	var semanaDados struct {
		Dados struct {
			Gasto int64 `json:"gasto_centavos"`
		}
	}
	a.exigir("GET", "/api/relatorios/"+semanal, nil, http.StatusOK).json(t, &semanaDados)
	if semanaDados.Dados.Gasto < 2000 {
		t.Fatalf("semana: %+v", semanaDados)
	}

	// avisa no início, e rodar de novo não cria outro
	var alertas []struct{ Tipo string }
	a.exigir("GET", "/api/alertas", nil, http.StatusOK).json(t, &alertas)
	n := 0
	for _, x := range alertas {
		if x.Tipo == "relatorio_pronto" {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("alertas: %+v", alertas)
	}
	var gerados struct{ Novos []string }
	a.exigir("POST", "/api/relatorios/gerar", nil, http.StatusOK).json(t, &gerados)
	if len(gerados.Novos) != 0 {
		t.Fatalf("repetiu: %+v", gerados)
	}
	a.exigir("GET", "/api/relatorios/00000000-0000-0000-0000-000000000000", nil, http.StatusNotFound)
}
