package api_test

import (
	"context"
	"math"
	"net/http"
	"testing"

	"github.com/yarion1/pi-finance/internal/financeiro"
)

// A carteira com um CDB a 100 % do CDI acompanha a linha do CDI mês a mês.
func TestHistoricoInvestimentosEAlvo(t *testing.T) {
	amb, a, pf, _, _ := prepara(t)
	ctx := context.Background()
	hoje := financeiro.Hoje()
	inicio := hoje.AddDate(0, -8, 0)
	if _, err := amb.banco.Dono.Exec(ctx, `insert into indices (serie, data, valor)
		select 'cdi', d, 0.055131 from generate_series($1::date, $2::date, '1 day') d where extract(isodow from d) < 6`,
		inicio.AddDate(0, 0, -10), hoje); err != nil {
		t.Fatal(err)
	}
	if _, err := amb.banco.Dono.Exec(ctx, `insert into cotacoes (codigo, data, preco, moeda, fonte)
		values ('^BVSP', $1, 100000, 'BRL', 'teste'), ('^BVSP', $2, 110000, 'BRL', 'teste')`,
		hoje.AddDate(0, -7, 0), hoje.AddDate(0, 0, -1)); err != nil {
		t.Fatal(err)
	}
	var cdb struct{ ID string }
	a.exigir("POST", "/api/investimentos/ativos", map[string]any{"entidade_id": pf, "codigo": "CDB 100",
		"classe": "renda_fixa", "indexador": "cdi", "taxa": "1"}, http.StatusCreated).json(t, &cdb)
	a.exigir("POST", "/api/investimentos/ativos/"+cdb.ID+"/operacoes", map[string]any{"data": inicio.Format("2006-01-02"),
		"tipo": "compra", "quantidade": "1", "preco": "10000"}, http.StatusCreated)

	var h struct {
		Datas     []string
		Valores   []int64 `json:"valores_centavos"`
		Carteira  []float64
		CDI       []*float64
		IPCA      []*float64
		Ibovespa  []*float64
		Proventos []struct {
			Mes   string
			Valor int64 `json:"valor_centavos"`
		}
	}
	a.exigir("GET", "/api/investimentos/historico?meses=6", nil, http.StatusOK).json(t, &h)
	n := len(h.Datas)
	if n != 7 || h.Valores[0] <= 1000000 || h.CDI[n-1] == nil || h.IPCA[n-1] != nil || h.Ibovespa[n-1] == nil ||
		math.Abs(*h.Ibovespa[n-1]-0.1) > 1e-9 {
		t.Fatalf("histórico: %+v", h)
	}
	// o juro pago sai da carteira como fluxo: a cota não cai por causa dele
	if d := math.Abs(h.Carteira[n-1]-*h.CDI[n-1]) * 100; d > 0.01 {
		t.Fatalf("100 %% do CDI: carteira %.6f × CDI %.6f (%.4f p.p.)", h.Carteira[n-1], *h.CDI[n-1], d)
	}

	// juro pago é retorno a mais (sai da carteira como fluxo) e aparece nos proventos
	a.exigir("POST", "/api/investimentos/ativos/"+cdb.ID+"/operacoes", map[string]any{"data": hoje.AddDate(0, -2, 0).Format("2006-01-02"),
		"tipo": "juros", "valor_centavos": 5000}, http.StatusCreated)
	cdi := *h.CDI[n-1]
	a.exigir("GET", "/api/investimentos/historico?meses=6", nil, http.StatusOK).json(t, &h)
	if h.Carteira[n-1] <= cdi+0.004 {
		t.Fatalf("com o juro, acima do CDI: %.6f × %.6f", h.Carteira[n-1], cdi)
	}
	var proventos int64
	for _, p := range h.Proventos {
		proventos += p.Valor
	}
	if len(h.Proventos) != 6 || proventos != 5000 {
		t.Fatalf("proventos: %+v", h.Proventos)
	}
	a.exigir("GET", "/api/investimentos/historico?meses=7", nil, http.StatusBadRequest)

	// alocação alvo: soma 100 %, classes conhecidas
	a.exigir("PUT", "/api/investimentos/alvo", map[string]any{"entidade_id": pf, "alvo": map[string]string{"acao": "0.6", "renda_fixa": "0.3"}},
		http.StatusBadRequest)
	a.exigir("PUT", "/api/investimentos/alvo", map[string]any{"entidade_id": pf, "alvo": map[string]string{"acao": "0.6", "xpto": "0.4"}},
		http.StatusBadRequest)
	a.exigir("PUT", "/api/investimentos/alvo", map[string]any{"entidade_id": pf, "alvo": map[string]string{"acao": "0.6", "renda_fixa": "0.4"}},
		http.StatusNoContent)
	var alvo struct {
		EntidadeID string            `json:"entidade_id"`
		Alvo       map[string]string `json:"alvo"`
	}
	a.exigir("GET", "/api/investimentos/alvo", nil, http.StatusOK).json(t, &alvo)
	if alvo.EntidadeID != pf || alvo.Alvo["acao"] != "0.6000" || alvo.Alvo["renda_fixa"] != "0.4000" {
		t.Fatalf("alvo: %+v", alvo)
	}
	a.exigir("PUT", "/api/investimentos/alvo", map[string]any{"entidade_id": pf, "alvo": map[string]string{}}, http.StatusNoContent)
	alvo.Alvo = nil // o decodificador junta num mapa que já existe
	a.exigir("GET", "/api/investimentos/alvo", nil, http.StatusOK).json(t, &alvo)
	if len(alvo.Alvo) != 0 {
		t.Fatalf("alvo apagado: %+v", alvo)
	}
}
