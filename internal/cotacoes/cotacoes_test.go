package cotacoes_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yarion1/pi-finance/internal/cotacoes"
	"github.com/yarion1/pi-finance/internal/db/dbteste"
)

// fontesFalsas responde como brapi, CoinGecko e SGS, e guarda os pedidos.
type fontesFalsas struct {
	mu      sync.Mutex
	pedidos []string
}

func (f *fontesFalsas) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.pedidos = append(f.pedidos, r.URL.String())
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.URL.Path == "/api/quote/PETR4":
		if r.URL.Query().Get("token") != "tk" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"results":[{"symbol":"PETR4","currency":"BRL","regularMarketPrice":38.47,"regularMarketTime":"2026-09-25T20:07:00.000Z"}]}`)
	case r.URL.Path == "/api/quote/HGLG11":
		fmt.Fprint(w, `{"results":[{"symbol":"HGLG11","currency":"BRL","regularMarketPrice":160.1,"regularMarketTime":"2026-09-24T21:00:00.000Z"}]}`)
	case strings.HasPrefix(r.URL.Path, "/api/quote/"):
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"error":true,"message":"Não encontramos a ação"}`)
	case r.URL.Path == "/api/v3/simple/price":
		if r.URL.Query().Get("ids") != "bitcoin,pepe" || r.URL.Query().Get("vs_currencies") != "brl" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		fmt.Fprint(w, `{"bitcoin":{"brl":612345.67,"last_updated_at":1790380800},"pepe":{"brl":6.2e-05}}`)
	case r.URL.Path == "/dados/serie/bcdata.sgs.12/dados":
		if r.URL.Query().Get("dataInicial") == "25/09/2026" {
			w.WriteHeader(http.StatusNotFound) // nada novo: o SGS responde 404
			return
		}
		fmt.Fprint(w, `[{"data":"23/09/2026","valor":"0.055131"},{"data":"24/09/2026","valor":"0.055131"}]`)
	case r.URL.Path == "/dados/serie/bcdata.sgs.433/dados":
		fmt.Fprint(w, `[{"data":"01/08/2026","valor":"0.25"},{"data":"01/09/2026","valor":"-0,11"}]`)
	case r.URL.Path == "/dados/serie/bcdata.sgs.1/dados":
		w.WriteHeader(http.StatusInternalServerError)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func TestAtualizar(t *testing.T) {
	ctx := context.Background()
	banco := dbteste.Novo(t)
	u := banco.CriarUsuario(t, "Ana", "ana@teste.com")
	var ent string
	if err := banco.Dono.QueryRow(ctx, "insert into entidades (dono_id, tipo, nome) values ($1, 'PF', 'Ana') returning id", u).Scan(&ent); err != nil {
		t.Fatal(err)
	}
	for _, a := range [][3]string{{"PETR4", "acao", "PETR4"}, {"PETR4-2", "acao", "PETR4"}, {"HGLG11", "fii", "HGLG11"},
		{"XPTO3", "acao", "XPTO3"}, {"Bitcoin", "cripto", "bitcoin"}, {"Pepe", "cripto", "pepe"}, {"CDB", "renda_fixa", ""}} {
		if _, err := banco.Dono.Exec(ctx, `insert into ativos (entidade_id, codigo, classe, cotacao_codigo)
			values ($1, $2, $3::classe_ativo, nullif($4, ''))`, ent, a[0], a[1], a[2]); err != nil {
			t.Fatal(err)
		}
	}

	falsa := &fontesFalsas{}
	srv := httptest.NewServer(falsa)
	defer srv.Close()
	f := &cotacoes.Fontes{HTTP: srv.Client(), Brapi: srv.URL, BrapiToken: "tk", CoinGecko: srv.URL, BCB: srv.URL,
		Agora: func() time.Time { return time.Date(2026, 9, 25, 22, 0, 0, 0, time.UTC) }}

	// o worker usa o papel do app, sem usuário na sessão
	rel, err := cotacoes.Atualizar(ctx, banco.App, f)
	if err != nil {
		t.Fatal(err)
	}
	if rel.Cotacoes != 4 || rel.Indices != 4 || len(rel.Erros) != 2 {
		t.Fatalf("relatório: %+v", rel)
	}
	var n int
	for _, c := range []struct{ codigo, data, preco, fonte string }{
		{"PETR4", "2026-09-25", "38.47000000", "brapi"},
		{"HGLG11", "2026-09-24", "160.10000000", "brapi"},
		{"bitcoin", "2026-09-25", "612345.67000000", "coingecko"},
		{"pepe", "2026-09-25", "0.00006200", "coingecko"},
	} {
		var data, preco, fonte string
		if err := banco.Dono.QueryRow(ctx, "select to_char(data, 'YYYY-MM-DD'), preco::text, fonte from cotacoes where codigo = $1", c.codigo).
			Scan(&data, &preco, &fonte); err != nil {
			t.Fatalf("%s: %v", c.codigo, err)
		}
		if data != c.data || preco != c.preco || fonte != c.fonte {
			t.Fatalf("%s: %s %s %s", c.codigo, data, preco, fonte)
		}
	}
	if err := banco.Dono.QueryRow(ctx, "select count(*) from indices where serie = 'ipca' and valor < 0").Scan(&n); err != nil || n != 1 {
		t.Fatalf("IPCA negativo com vírgula: %d %v", n, err)
	}
	// o código sai uma vez só, mesmo em dois ativos; renda fixa sem código não é cotada
	falsa.mu.Lock()
	petr := 0
	for _, p := range falsa.pedidos {
		if strings.HasPrefix(p, "/api/quote/PETR4") {
			petr++
		}
		if strings.Contains(p, "CDB") {
			t.Fatalf("pedido indevido: %s", p)
		}
	}
	falsa.pedidos = nil
	falsa.mu.Unlock()
	if petr != 1 {
		t.Fatalf("PETR4 pedida %d vezes", petr)
	}

	// segunda rodada: os índices continuam do dia seguinte ao último guardado
	if _, err := cotacoes.Atualizar(ctx, banco.App, f); err != nil {
		t.Fatal(err)
	}
	falsa.mu.Lock()
	defer falsa.mu.Unlock()
	achou := false
	for _, p := range falsa.pedidos {
		if strings.Contains(p, "sgs.12/") {
			achou = true
			if !strings.Contains(p, "dataInicial=25%2F09%2F2026") {
				t.Fatalf("CDI deveria continuar de 25/09: %s", p)
			}
		}
	}
	if !achou {
		t.Fatal("não pediu o CDI de novo")
	}
}

func TestFonteForaDoAr(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()
	f := cotacoes.Padrao("")
	f.HTTP, f.Brapi, f.CoinGecko, f.BCB = srv.Client(), srv.URL, srv.URL, srv.URL
	if _, err := f.Acao(context.Background(), "PETR4"); err == nil || !strings.Contains(err.Error(), "502") {
		t.Fatalf("erro esperado: %v", err)
	}
	if _, err := f.Cripto(context.Background(), []string{"bitcoin"}); err == nil {
		t.Fatal("erro esperado")
	}
	if _, err := f.SerieBCB(context.Background(), "selic", time.Now(), time.Now()); err == nil {
		t.Fatal("série desconhecida")
	}
	if x, err := f.Cripto(context.Background(), nil); err != nil || x != nil {
		t.Fatal("sem ids, sem pedido")
	}
}
