package pluggy_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/pluggy"
	"github.com/yarion1/pi-finance/internal/pluggy/pluggyfalsa"
)

func tx(id, data, valor, tipo string) pluggy.Transacao {
	return pluggy.Transacao{ID: id, Date: data, Amount: json.Number(valor), Type: tipo, Status: "POSTED", Description: "Mercado " + id}
}

func TestCentavosSemFloat(t *testing.T) {
	casos := map[string]core.Centavos{"-45.9": -4590, "1234.56": 123456, "0.1": 10, "10": 1000, "+3.005": 301, "-3.005": -301}
	for entrada, esperado := range casos {
		got, err := pluggy.Centavos(json.Number(entrada))
		if err != nil || got != esperado {
			t.Errorf("%s: %d, %v (esperado %d)", entrada, got, err, esperado)
		}
	}
	for _, ruim := range []string{"1e3", "", "abc"} {
		if _, err := pluggy.Centavos(json.Number(ruim)); err == nil {
			t.Errorf("%q deveria falhar", ruim)
		}
	}
}

func TestDataSP(t *testing.T) {
	casos := map[string]string{
		"2026-09-20":                "2026-09-20",
		"2026-09-20T03:00:00.000Z":  "2026-09-20", // meia-noite em São Paulo
		"2026-09-20T02:59:00.000Z":  "2026-09-19",
		"2026-09-20T10:00:00-03:00": "2026-09-20",
	}
	for entrada, esperado := range casos {
		d, err := pluggy.DataSP(entrada)
		if err != nil || d.Format("2006-01-02") != esperado {
			t.Errorf("%s: %v %v (esperado %s)", entrada, d, err, esperado)
		}
	}
	if _, err := pluggy.DataSP("ontem"); err == nil {
		t.Error("data ruim deveria falhar")
	}
}

func TestLinhaSinalEParcela(t *testing.T) {
	casos := []struct {
		nome   string
		t      pluggy.Transacao
		cartao bool
		valor  core.Centavos
	}{
		{"débito na conta", tx("1", "2026-09-10", "-50.00", "DEBIT"), false, -5000},
		{"débito com valor positivo", tx("2", "2026-09-10", "50.00", "DEBIT"), false, -5000},
		{"crédito na conta", tx("3", "2026-09-10", "1200", "CREDIT"), false, 120000},
		{"compra no cartão vem positiva", tx("4", "2026-09-10", "89.90", "DEBIT"), true, -8990},
		{"pagamento do cartão vem negativo", tx("5", "2026-09-10", "-500", "CREDIT"), true, 50000},
		{"sem tipo usa o sinal", tx("6", "2026-09-10", "-7", ""), false, -700},
	}
	for _, c := range casos {
		l, ok, err := c.t.Linha(c.cartao)
		if err != nil || !ok || l.Valor != c.valor || l.IDExterno != c.t.ID {
			t.Errorf("%s: %+v ok=%v err=%v", c.nome, l, ok, err)
		}
	}

	pendente := tx("7", "2026-09-10", "10", "DEBIT")
	pendente.Status = "PENDING"
	if _, ok, err := pendente.Linha(false); ok || err != nil {
		t.Error("pendente não entra")
	}

	// parcela 3/10 de uma compra de 20/07: data do mês da parcela e "(3/10)" na descrição
	p := tx("8", "2026-07-20T12:00:00.000Z", "100", "DEBIT")
	p.Description = "Loja X"
	p.CreditCardMetadata = &struct {
		InstallmentNumber int    `json:"installmentNumber"`
		TotalInstallments int    `json:"totalInstallments"`
		PurchaseDate      string `json:"purchaseDate"`
	}{3, 10, "2026-07-20T12:00:00.000Z"}
	l, _, _ := p.Linha(true)
	if l.Descricao != "Loja X (3/10)" || l.Data.Format("2006-01-02") != "2026-09-20" || l.Valor != -10000 {
		t.Errorf("parcela: %+v", l)
	}
	// descrição que já traz a parcela não ganha outra
	p.Description = "Loja X PARC 03/10"
	if l, _, _ := p.Linha(true); l.Descricao != "Loja X PARC 03/10" {
		t.Errorf("parcela repetida: %q", l.Descricao)
	}
	// sem descrição: a bruta; sem nenhuma: um texto fixo
	s := tx("9", "2026-09-10", "1", "DEBIT")
	s.Description, s.DescriptionRaw = "", "PIX QRS"
	if l, _, _ := s.Linha(false); l.Descricao != "PIX QRS" {
		t.Errorf("descrição bruta: %q", l.Descricao)
	}
	s.DescriptionRaw = ""
	if l, _, _ := s.Linha(false); l.Descricao != "Sem descrição" {
		t.Errorf("sem descrição: %q", l.Descricao)
	}
	ruim := tx("10", "hoje", "1", "DEBIT")
	if _, _, err := ruim.Linha(false); err == nil {
		t.Error("data ruim deveria falhar")
	}
	ruim = tx("11", "2026-09-10", "1e2", "DEBIT")
	if _, _, err := ruim.Linha(false); err == nil {
		t.Error("valor ruim deveria falhar")
	}
}

func TestConta(t *testing.T) {
	c := pluggy.Conta{ID: "c1", Type: "CREDIT", Name: "Cartão", MarketingName: " Nubank Ultravioleta ", Balance: "1500.10", CurrencyCode: "brl"}
	c.CreditData = &pluggy.DadosCartao{CreditLimit: "8000", AvailableCreditLimit: "6500.50", BalanceCloseDate: "2026-10-03T03:00:00.000Z", BalanceDueDate: "2026-10-10"}
	if !c.Cartao() || c.TipoConta() != "cartao" || c.NomeExibido() != "Nubank Ultravioleta" || c.Moeda() != "BRL" {
		t.Errorf("cartão: %v %s %q %s", c.Cartao(), c.TipoConta(), c.NomeExibido(), c.Moeda())
	}
	if s, _ := c.Saldo(); s != -150010 {
		t.Errorf("saldo do cartão é dívida: %d", s)
	}
	if l := c.Limite(); l == nil || *l != 800000 {
		t.Errorf("limite: %v", l)
	}
	if f, v := c.DiasDoCartao(); f == nil || *f != 3 || v == nil || *v != 10 {
		t.Errorf("dias: %v %v", f, v)
	}

	b := pluggy.Conta{ID: "b1", Type: "BANK", Subtype: "SAVINGS_ACCOUNT", Number: "123", Balance: "10"}
	if b.TipoConta() != "poupanca" || b.NomeExibido() != "123" || b.Limite() != nil || b.Moeda() != "BRL" {
		t.Errorf("poupança: %s %q", b.TipoConta(), b.NomeExibido())
	}
	if f, v := b.DiasDoCartao(); f != nil || v != nil {
		t.Error("conta sem dias de cartão")
	}
	if s, _ := b.Saldo(); s != 1000 {
		t.Errorf("saldo: %d", s)
	}
	b.Subtype, b.Number = "CHECKING_ACCOUNT", ""
	if b.TipoConta() != "corrente" || b.NomeExibido() != "b1" {
		t.Errorf("corrente: %s %q", b.TipoConta(), b.NomeExibido())
	}
	b.Balance = "x"
	if _, err := b.Saldo(); err == nil {
		t.Error("saldo ruim deveria falhar")
	}
}

func TestItemProblema(t *testing.T) {
	i := pluggy.Item{Status: "UPDATED", LastUpdatedAt: "2026-09-25T09:00:00.000Z"}
	if i.Problema() != "" || i.AtualizadoEm() == nil {
		t.Error("item em dia")
	}
	for _, s := range []string{"LOGIN_ERROR", "WAITING_USER_INPUT", "OUTDATED"} {
		i.Status = s
		if i.Problema() == "" {
			t.Errorf("%s deveria ter problema", s)
		}
	}
	i.Error = &struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{"X", "site do banco fora"}
	if i.Problema() != "desatualizado: site do banco fora" {
		t.Error(i.Problema())
	}
	i.LastUpdatedAt = ""
	if i.AtualizadoEm() != nil {
		t.Error("sem data")
	}
}

func TestClienteContraAFalsa(t *testing.T) {
	f := pluggyfalsa.Nova()
	f.Cliente("ana", "segredo")
	f.Cliente("bia", "outro")
	conta := &pluggyfalsa.Conta{Conta: pluggy.Conta{ID: "acc-1", Type: "BANK", Name: "Conta", Balance: "100"}}
	for i, d := range []string{"2026-09-01", "2026-09-02", "2026-09-03", "2026-09-04", "2026-09-05"} {
		conta.Transacoes = append(conta.Transacoes, tx(string(rune('a'+i)), d, "-1", "DEBIT"))
	}
	f.Item("ana", "item-1", "Nubank", conta)
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := pluggy.Novo(srv.URL + "/")
	ctx := context.Background()

	if _, err := c.Autenticar(ctx, "ana", "errado"); !errors.Is(err, pluggy.ErrCredenciais) {
		t.Fatalf("senha errada: %v", err)
	}
	chave, err := c.Autenticar(ctx, "ana", "segredo")
	if err != nil {
		t.Fatal(err)
	}
	it, err := c.Item(ctx, chave, "item-1")
	if err != nil || it.Connector.Name != "Nubank" || it.Status != "UPDATED" {
		t.Fatalf("item: %+v %v", it, err)
	}
	contas, err := c.Contas(ctx, chave, "item-1")
	if err != nil || len(contas) != 1 || contas[0].ID != "acc-1" {
		t.Fatalf("contas: %+v %v", contas, err)
	}
	// 5 transações em páginas de 2: o cursor precisa ser seguido
	lista, err := c.Transacoes(ctx, chave, "acc-1", time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
	if err != nil || len(lista) != 4 {
		t.Fatalf("transações: %d %v", len(lista), err)
	}

	// se a v2 recusar, a v1 (páginas numeradas) traz as mesmas transações
	f.SemV2 = true
	lista, err = c.Transacoes(ctx, chave, "acc-1", time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC))
	if err != nil || len(lista) != 4 {
		t.Fatalf("transações pela v1: %d %v", len(lista), err)
	}
	f.SemV2 = false

	// outra pessoa não vê o item de Ana
	chaveBia, _ := c.Autenticar(ctx, "bia", "outro")
	if _, err := c.Item(ctx, chaveBia, "item-1"); !errors.Is(err, pluggy.ErrItemNaoEncontrado) {
		t.Errorf("bia vendo item de ana: %v", err)
	}
	if _, err := c.Contas(ctx, "chave-invalida", "item-1"); !errors.Is(err, pluggy.ErrCredenciais) {
		t.Errorf("chave inválida: %v", err)
	}

	// servidor fora e resposta quebrada
	quebrado := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/items/recusado" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"code":400,"message":"parâmetro inválido: dateFrom"}`))
			return
		}
		if r.URL.Path == "/items/recusado-sem-json" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.URL.Path == "/auth" {
			_, _ = w.Write([]byte(`{"apiKey": ""}`))
			return
		}
		if r.URL.Path == "/accounts" {
			_, _ = w.Write([]byte(`não é json`))
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer quebrado.Close()
	q := pluggy.Novo(quebrado.URL)
	if _, err := q.Autenticar(ctx, "a", "b"); !errors.Is(err, pluggy.ErrCredenciais) {
		t.Errorf("chave vazia: %v", err)
	}
	// 400 traz a mensagem da Pluggy, para dar para entender o que houve
	if _, err := q.Item(ctx, "k", "recusado"); !errors.Is(err, pluggy.ErrRecusada) || !strings.Contains(err.Error(), "parâmetro inválido: dateFrom") {
		t.Errorf("400: %v", err)
	}
	if _, err := q.Item(ctx, "k", "recusado-sem-json"); !errors.Is(err, pluggy.ErrRecusada) || !strings.Contains(err.Error(), "sem detalhes") {
		t.Errorf("400 sem corpo: %v", err)
	}
	if _, err := q.Item(ctx, "k", "x"); !errors.Is(err, pluggy.ErrIndisponivel) {
		t.Errorf("500: %v", err)
	}
	if _, err := q.Contas(ctx, "k", "x"); !errors.Is(err, pluggy.ErrIndisponivel) {
		t.Errorf("json quebrado: %v", err)
	}
	if _, err := pluggy.Novo("http://127.0.0.1:1").Autenticar(ctx, "a", "b"); !errors.Is(err, pluggy.ErrIndisponivel) {
		t.Errorf("fora do ar: %v", err)
	}
	if pluggy.Novo("").Base != pluggy.URLPadrao {
		t.Error("base padrão")
	}
}
