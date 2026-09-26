package api_test

import (
	"net/http"
	"strings"
	"testing"
)

func TestProjecoesESimulacoes(t *testing.T) {
	amb, a, pf, nu, _ := prepara(t)
	d := func(n int) string { return strings.ReplaceAll(dia(n), "-", "") }
	// três meses de gasto variável de R$ 200 e um salário que não se repete
	importarOFX(t, a, nu, ofx(
		[4]string{d(-95), "-200.00", "g1", "LOJA A"},
		[4]string{d(-65), "-200.00", "g2", "LOJA B"},
		[4]string{d(-35), "-200.00", "g3", "LOJA C"},
		[4]string{d(-60), "5000.00", "s1", "SALARIO"},
	))

	var fluxo struct {
		SaldoInicial int64 `json:"saldo_inicial_centavos"`
		Pontos       []struct {
			Base  int64 `json:"base_centavos"`
			Baixo int64 `json:"baixo_centavos"`
		} `json:"pontos"`
		Final struct {
			Base int64 `json:"base_centavos"`
		} `json:"final"`
	}
	a.exigir("GET", "/api/projecoes/fluxo?dias=60", nil, http.StatusOK).json(t, &fluxo)
	if fluxo.SaldoInicial != 540000 || len(fluxo.Pontos) != 61 || fluxo.Final.Base >= fluxo.SaldoInicial ||
		fluxo.Final.Base < fluxo.SaldoInicial-50000 {
		t.Fatalf("fluxo: saldo %d final %d (%d pontos)", fluxo.SaldoInicial, fluxo.Final.Base, len(fluxo.Pontos))
	}
	a.exigir("GET", "/api/projecoes/fluxo?dias=400", nil, http.StatusBadRequest)
	a.exigir("GET", "/api/projecoes/fluxo?dias=x", nil, http.StatusBadRequest)

	type compra struct {
		Parcela  int64  `json:"parcela_centavos"`
		Situacao string `json:"situacao"`
		Ultima   string `json:"ultima_data"`
	}
	var c compra
	a.exigir("POST", "/api/simulacoes/compra", map[string]any{"valor_centavos": 6000, "parcelas": 3, "primeira": dia(10)}, http.StatusOK).json(t, &c)
	if c.Situacao != "folgada" || c.Parcela != 2000 {
		t.Fatalf("compra pequena: %+v", c)
	}
	a.exigir("POST", "/api/simulacoes/compra", map[string]any{"valor_centavos": 600000, "parcelas": 1, "primeira": dia(5)}, http.StatusOK).json(t, &c)
	if c.Situacao != "nao_cabe" {
		t.Fatalf("compra grande: %+v", c)
	}
	a.exigir("POST", "/api/simulacoes/compra", map[string]any{"valor_centavos": 1000, "parcelas": 3, "primeira": dia(-1)}, http.StatusBadRequest)
	a.exigir("POST", "/api/simulacoes/compra", map[string]any{"valor_centavos": 1000, "parcelas": 30, "primeira": dia(1)}, http.StatusBadRequest)
	a.exigir("POST", "/api/simulacoes/compra", map[string]any{"valor_centavos": 1000, "parcelas": 3, "primeira": "ontem"}, http.StatusBadRequest)

	var fin struct {
		Compensa      bool    `json:"compensa_parcelar"`
		RendimentoMes float64 `json:"rendimento_mes"`
		DoCDI         bool    `json:"do_cdi"`
	}
	a.exigir("POST", "/api/simulacoes/financiamento", map[string]any{"avista_centavos": 100000, "parcela_centavos": 10000,
		"parcelas": 10, "primeira_em_meses": 1}, http.StatusOK).json(t, &fin)
	if !fin.Compensa || !fin.DoCDI || fin.RendimentoMes <= 0 {
		t.Fatalf("financiamento: %+v", fin)
	}
	a.exigir("POST", "/api/simulacoes/financiamento", map[string]any{"avista_centavos": 90000, "parcela_centavos": 10000,
		"parcelas": 10, "primeira_em_meses": 1, "rendimento_mes": 0.01}, http.StatusOK).json(t, &fin)
	if fin.Compensa || fin.DoCDI {
		t.Fatalf("financiamento com juros: %+v", fin)
	}
	a.exigir("POST", "/api/simulacoes/financiamento", map[string]any{"avista_centavos": 0, "parcela_centavos": 1, "parcelas": 1}, http.StatusBadRequest)
	a.exigir("POST", "/api/simulacoes/financiamento", map[string]any{"avista_centavos": 1, "parcela_centavos": 1, "parcelas": 1, "rendimento_mes": 0.5}, http.StatusBadRequest)

	var divida struct{ ID string }
	a.exigir("POST", "/api/dividas", map[string]any{"entidade_id": pf, "nome": "Carro", "tipo": "financiamento", "sistema": "price",
		"principal_centavos": 1000000, "taxa_mensal": "0.01", "prazo_meses": 12, "primeiro_vencimento": dia(10)}, http.StatusCreated).json(t, &divida)
	var q struct {
		Restantes int   `json:"parcelas_restantes"`
		Novas     int   `json:"novas_parcelas"`
		Economia  int64 `json:"economia_centavos"`
	}
	a.exigir("POST", "/api/simulacoes/quitar", map[string]any{"divida_id": divida.ID, "valor_centavos": 300000, "modo": "prazo"}, http.StatusOK).json(t, &q)
	if q.Restantes != 12 || q.Novas >= 12 || q.Economia <= 0 {
		t.Fatalf("quitar: %+v", q)
	}
	a.exigir("POST", "/api/simulacoes/quitar", map[string]any{"divida_id": divida.ID, "valor_centavos": 300000, "modo": "x"}, http.StatusBadRequest)

	var fut struct {
		Classes []struct{ Classe string } `json:"classes"`
		Pontos  []struct {
			P10 int64 `json:"p10_centavos"`
			P50 int64 `json:"p50_centavos"`
			P90 int64 `json:"p90_centavos"`
		} `json:"pontos"`
		Cenarios []struct {
			Nome  string
			Meses int
		} `json:"cenarios"`
		Alvo int64 `json:"alvo_centavos"`
	}
	a.exigir("GET", "/api/projecoes/futuro?aporte=100000&anos=5&perfil=moderado&r_acao=8,5", nil, http.StatusBadRequest)
	a.exigir("GET", "/api/projecoes/futuro?aporte=100000&anos=5&perfil=moderado&r_acao=8.5&v_acao=30", nil, http.StatusOK).json(t, &fut)
	if len(fut.Pontos) != 5 || len(fut.Cenarios) != 3 || fut.Alvo <= 0 || len(fut.Classes) < 2 ||
		!(fut.Pontos[4].P10 < fut.Pontos[4].P50 && fut.Pontos[4].P50 < fut.Pontos[4].P90) {
		t.Fatalf("futuro: %+v", fut)
	}
	a.exigir("GET", "/api/projecoes/futuro?perfil=xpto", nil, http.StatusBadRequest)
	a.exigir("GET", "/api/projecoes/futuro?anos=0", nil, http.StatusBadRequest)
	a.exigir("GET", "/api/projecoes/futuro?v_acao=500", nil, http.StatusBadRequest)

	// outra pessoa não simula com a dívida de Ana
	b := amb.novoCliente()
	var casa struct{ ID string }
	a.exigir("POST", "/api/casas", map[string]string{"nome": "Casa"}, http.StatusCreated).json(t, &casa)
	var convite struct{ Link string }
	a.exigir("POST", "/api/casas/"+casa.ID+"/convites", map[string]string{"papel": "membro"}, http.StatusCreated).json(t, &convite)
	b.cadastrarCom2FA("Bruno", "bruno@teste.com", convite.Link[strings.LastIndex(convite.Link, "/")+1:])
	b.exigir("POST", "/api/simulacoes/quitar", map[string]any{"divida_id": divida.ID, "valor_centavos": 1, "modo": "prazo"}, http.StatusNotFound)
	b.exigir("POST", "/api/simulacoes/compra", map[string]any{"entidade_id": pf, "valor_centavos": 1, "parcelas": 1, "primeira": dia(1)}, http.StatusNotFound)
}
