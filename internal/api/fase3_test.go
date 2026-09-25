package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yarion1/pi-finance/internal/financeiro"
	"github.com/yarion1/pi-finance/internal/pluggy"
	"github.com/yarion1/pi-finance/internal/pluggy/pluggyfalsa"
	"github.com/yarion1/pi-finance/internal/worker"
)

// dia relativo a hoje em São Paulo, no formato da Pluggy (instante em UTC).
func diaPluggy(dias int) string {
	return financeiro.Hoje().AddDate(0, 0, dias).Add(15 * time.Hour).Format("2006-01-02T15:04:05.000Z")
}

func dia(dias int) string { return financeiro.Hoje().AddDate(0, 0, dias).Format("2006-01-02") }

func txPluggy(id string, dias int, valor, tipo, descricao string) pluggy.Transacao {
	return pluggy.Transacao{ID: id, Date: diaPluggy(dias), Amount: json.Number(valor), Type: tipo, Status: "POSTED", Description: descricao}
}

func contaBanco(id, nome, saldo string, txs ...pluggy.Transacao) *pluggyfalsa.Conta {
	return &pluggyfalsa.Conta{
		Conta:      pluggy.Conta{ID: id, Type: "BANK", Subtype: "CHECKING_ACCOUNT", Name: nome, Balance: json.Number(saldo), CurrencyCode: "BRL"},
		Transacoes: txs,
	}
}

type estadoOF struct {
	Conexao *struct {
		ClientIDFim string  `json:"client_id_fim"`
		UltimoErro  *string `json:"ultimo_erro"`
	} `json:"conexao"`
	Itens []struct {
		ID          string  `json:"id"`
		Instituicao *string `json:"instituicao"`
		UltimoErro  *string `json:"ultimo_erro"`
		Contas      []struct {
			ID      string  `json:"id"`
			Nome    string  `json:"nome"`
			Saldo   *int64  `json:"saldo_centavos"`
			ContaID *string `json:"conta_id"`
		} `json:"contas"`
	} `json:"itens"`
}

type relatorioOF struct {
	Novas        int      `json:"novas"`
	Casadas      int      `json:"casadas"`
	Divergencias int      `json:"divergencias"`
	Pendentes    int      `json:"contas_sem_vinculo"`
	Erros        []string `json:"erros"`
}

// conectar cadastra as credenciais e um banco para a pessoa.
func conectar(t *testing.T, c *cliente, clientID, secret, item, entidade string) (itemID string) {
	t.Helper()
	// credenciais pedem a senha de novo
	c.exigir("POST", "/api/auth/reautenticar", map[string]string{"senha": "senha comprida de teste"}, http.StatusNoContent)
	c.exigir("PUT", "/api/open-finance/credenciais", map[string]string{"client_id": clientID, "client_secret": secret}, http.StatusNoContent)
	var r struct{ ID string }
	c.exigir("POST", "/api/open-finance/itens", map[string]string{"item_id": item, "entidade_id": entidade}, http.StatusCreated).json(t, &r)
	return r.ID
}

// sincronizar força a sincronização (a trava de 10 s vale só para o botão).
func sincronizar(t *testing.T, amb *ambiente, usuario string) relatorioOF {
	t.Helper()
	rel, err := amb.of.Sincronizar(context.Background(), usuario)
	if err != nil {
		t.Fatal(err)
	}
	var r relatorioOF
	b, _ := json.Marshal(rel)
	_ = json.Unmarshal(b, &r)
	return r
}

func usuarioDe(t *testing.T, c *cliente) string {
	t.Helper()
	var s struct {
		ID string `json:"usuario_id"`
	}
	c.exigir("GET", "/api/auth/sessao", nil, http.StatusOK).json(t, &s)
	return s.ID
}

// Critério de aceite da fase 3: cada usuário conecta as próprias contas.
func TestCadaUmConectaAsProprias(t *testing.T) {
	amb, a, pfA, _, _ := prepara(t)
	amb.pluggy.Cliente("cliente-da-ana-1234", "segredo-da-ana")
	amb.pluggy.Cliente("cliente-do-bruno-5678", "segredo-do-bruno")
	amb.pluggy.Item("cliente-da-ana-1234", "item-ana", "Nubank",
		contaBanco("acc-ana", "Conta Ana", "900", txPluggy("t1", -3, "-100", "DEBIT", "MERCADO DA ANA")))
	amb.pluggy.Item("cliente-do-bruno-5678", "item-bruno", "Itaú",
		contaBanco("acc-bruno", "Conta Bruno", "50", txPluggy("t2", -2, "-10", "DEBIT", "PADARIA DO BRUNO")))

	// credenciais pedem a senha de novo; errada não é gravada
	a.exigir("PUT", "/api/open-finance/credenciais", map[string]string{"client_id": "cliente-da-ana-1234", "client_secret": "segredo-da-ana"}, http.StatusForbidden)
	a.exigir("POST", "/api/auth/reautenticar", map[string]string{"senha": "senha comprida de teste"}, http.StatusNoContent)
	a.exigir("PUT", "/api/open-finance/credenciais", map[string]string{"client_id": "cliente-da-ana-1234", "client_secret": "errado"}, http.StatusBadRequest)
	a.exigir("POST", "/api/open-finance/itens", map[string]string{"item_id": "item-ana", "entidade_id": pfA}, http.StatusBadRequest)
	item := conectar(t, a, "cliente-da-ana-1234", "segredo-da-ana", "item-ana", pfA)
	a.exigir("POST", "/api/open-finance/itens", map[string]string{"item_id": "item-ana", "entidade_id": pfA}, http.StatusConflict)
	// o item do Bruno não é das credenciais da Ana
	a.exigir("POST", "/api/open-finance/itens", map[string]string{"item_id": "item-bruno", "entidade_id": pfA}, http.StatusBadRequest)

	// o segredo nunca volta e fica cifrado no banco
	r := a.exigir("GET", "/api/open-finance", nil, http.StatusOK)
	if bytes.Contains(r.corpo, []byte("segredo-da-ana")) || bytes.Contains(r.corpo, []byte("cliente-da-ana")) {
		t.Fatalf("credencial exposta: %s", r.corpo)
	}
	var cru string
	if err := amb.banco.Dono.QueryRow(context.Background(), "select client_secret || client_id from conexoes_pluggy").Scan(&cru); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains([]byte(cru), []byte("segredo-da-ana")) || bytes.Contains([]byte(cru), []byte("cliente-da-ana")) {
		t.Fatal("credencial em texto puro no banco")
	}

	// a primeira sincronização traz as contas; sem vínculo, nada é importado
	a.exigir("POST", "/api/open-finance/sincronizar", nil, http.StatusOK)
	a.exigir("POST", "/api/open-finance/sincronizar", nil, http.StatusTooManyRequests) // botão com trava de 10 s
	var e estadoOF
	a.exigir("GET", "/api/open-finance", nil, http.StatusOK).json(t, &e)
	if e.Conexao == nil || e.Conexao.ClientIDFim != "1234" || len(e.Itens) != 1 || e.Itens[0].ID != item ||
		*e.Itens[0].Instituicao != "Nubank" || len(e.Itens[0].Contas) != 1 || e.Itens[0].Contas[0].ContaID != nil {
		t.Fatalf("estado: %+v", e)
	}

	// cria a conta no painel pela Pluggy e sincroniza: transações entram e o saldo bate com o banco
	contaPluggy := e.Itens[0].Contas[0].ID
	a.exigir("PATCH", "/api/open-finance/contas/"+contaPluggy, map[string]string{"acao": "criar"}, http.StatusNoContent)
	rel := sincronizar(t, amb, usuarioDe(t, a))
	if rel.Novas != 1 || rel.Divergencias != 0 || len(rel.Erros) != 0 {
		t.Fatalf("sincronização: %+v", rel)
	}
	var contas []struct {
		ID          string `json:"id"`
		Nome        string `json:"nome"`
		Saldo       int64  `json:"saldo_centavos"`
		SaldoBanco  *int64 `json:"saldo_banco_centavos"`
		OpenFinance bool   `json:"open_finance"`
		Instituicao *string
	}
	a.exigir("GET", "/api/contas", nil, http.StatusOK).json(t, &contas)
	var criada string
	for _, c := range contas {
		if c.Nome == "Conta Ana" {
			criada = c.ID
			if c.Saldo != 90000 || c.SaldoBanco == nil || *c.SaldoBanco != 90000 || !c.OpenFinance {
				t.Fatalf("conta criada: %+v", c)
			}
		}
	}
	if criada == "" {
		t.Fatalf("conta não criada: %+v", contas)
	}
	if r := a.exigir("GET", "/api/transacoes?conta_id="+criada, nil, http.StatusOK); !bytes.Contains(r.corpo, []byte("MERCADO DA ANA")) {
		t.Fatalf("transação da Pluggy: %s", r.corpo)
	}
	// de novo: nada novo, nada duplicado, e a importação vazia não fica no histórico
	if rel := sincronizar(t, amb, usuarioDe(t, a)); rel.Novas != 0 {
		t.Fatalf("segunda sincronização duplicou: %+v", rel)
	}
	var imps []struct{ Fonte string }
	a.exigir("GET", "/api/importacoes", nil, http.StatusOK).json(t, &imps)
	if len(imps) != 1 || imps[0].Fonte != "pluggy" {
		t.Fatalf("importações: %+v", imps)
	}

	// Bruno, da mesma casa, conecta as dele e não enxerga nada da Ana (nem ela as dele)
	var casa struct{ ID string }
	a.exigir("POST", "/api/casas", map[string]string{"nome": "Casa"}, http.StatusCreated).json(t, &casa)
	var convite struct{ Link string }
	a.exigir("POST", "/api/casas/"+casa.ID+"/convites", map[string]string{"papel": "membro"}, http.StatusCreated).json(t, &convite)
	b := amb.novoCliente()
	b.cadastrarCom2FA("Bruno", "bruno@teste.com", convite.Link[strings.LastIndex(convite.Link, "/")+1:])
	var pfB struct{ ID string }
	b.exigir("POST", "/api/entidades", map[string]any{"tipo": "PF", "nome": "Bruno PF"}, http.StatusCreated).json(t, &pfB)
	b.exigir("POST", "/api/open-finance/sincronizar", nil, http.StatusBadRequest) // sem credenciais
	b.exigir("POST", "/api/open-finance/itens", map[string]string{"item_id": "item-bruno", "entidade_id": pfB.ID}, http.StatusBadRequest)
	conectar(t, b, "cliente-do-bruno-5678", "segredo-do-bruno", "item-bruno", pfB.ID)
	// o item da Ana não vale com as credenciais do Bruno, nem mandando para a entidade dela
	b.exigir("POST", "/api/open-finance/itens", map[string]string{"item_id": "item-ana", "entidade_id": pfB.ID}, http.StatusBadRequest)
	b.exigir("POST", "/api/open-finance/itens", map[string]string{"item_id": "item-bruno", "entidade_id": pfA}, http.StatusNotFound)
	sincronizar(t, amb, usuarioDe(t, b))
	r = b.exigir("GET", "/api/open-finance", nil, http.StatusOK)
	for _, p := range []string{"Nubank", "Conta Ana", item, contaPluggy, "1234"} {
		if bytes.Contains(r.corpo, []byte(p)) {
			t.Errorf("Bruno viu %q da Ana: %s", p, r.corpo)
		}
	}
	if !bytes.Contains(r.corpo, []byte("Conta Bruno")) {
		t.Fatalf("Bruno deveria ver a própria conta: %s", r.corpo)
	}
	// e não mexe nas coisas da Ana
	b.exigir("PATCH", "/api/open-finance/contas/"+contaPluggy, map[string]string{"acao": "ignorar"}, http.StatusNotFound)
	b.exigir("DELETE", "/api/open-finance/itens/"+item, nil, http.StatusNotFound)
	var eb estadoOF
	b.exigir("GET", "/api/open-finance", nil, http.StatusOK).json(t, &eb)
	// vincular a conta do Bruno a uma conta da Ana: recusado
	b.exigir("PATCH", "/api/open-finance/contas/"+eb.Itens[0].Contas[0].ID, map[string]string{"acao": "vincular", "conta_id": criada}, http.StatusNotFound)
	b.exigir("POST", "/api/contas/"+criada+"/acertar-saldo", nil, http.StatusNotFound)

	// desconectar pede reautenticação e mantém as transações
	a.exigir("DELETE", "/api/open-finance", nil, http.StatusNoContent)
	a.exigir("GET", "/api/open-finance", nil, http.StatusOK).json(t, &e)
	if e.Conexao != nil || len(e.Itens) != 0 {
		t.Fatalf("desconectado: %+v", e)
	}
	if r := a.exigir("GET", "/api/transacoes?conta_id="+criada, nil, http.StatusOK); !bytes.Contains(r.corpo, []byte("MERCADO DA ANA")) {
		t.Fatal("as transações ficam depois de desconectar")
	}
}

// Critério de aceite da fase 3: diferença de saldo gera alerta.
func TestDiferencaDeSaldoGeraAlerta(t *testing.T) {
	amb, a, pf, nu, _ := prepara(t)
	amb.pluggy.Cliente("cli", "sec")
	amb.pluggy.Item("cli", "item", "Nubank", contaBanco("acc", "Nubank", "1000.00",
		txPluggy("p1", -5, "-45.90", "DEBIT", "PADARIA PAO QUENTE LTDA"),
		txPluggy("p2", -1, "20", "CREDIT", "PIX RECEBIDO")))
	conectar(t, a, "cli", "sec", "item", pf)
	u := usuarioDe(t, a)
	sincronizar(t, amb, u)
	var e estadoOF
	a.exigir("GET", "/api/open-finance", nil, http.StatusOK).json(t, &e)

	// liga à conta Nubank que já existe (saldo inicial R$ 1.000,00): 1000 − 45,90 + 20 = 974,10 ≠ 1000
	a.exigir("PATCH", "/api/open-finance/contas/"+e.Itens[0].Contas[0].ID, map[string]string{"acao": "vincular", "conta_id": nu}, http.StatusNoContent)
	rel := sincronizar(t, amb, u)
	if rel.Novas != 2 || rel.Divergencias != 1 {
		t.Fatalf("sincronização: %+v", rel)
	}
	var alertas []struct {
		Tipo  string         `json:"tipo"`
		Dados map[string]any `json:"dados"`
	}
	a.exigir("GET", "/api/alertas", nil, http.StatusOK).json(t, &alertas)
	if len(alertas) != 1 || alertas[0].Tipo != "saldo_divergente" || alertas[0].Dados["diferenca_centavos"] != float64(2590) ||
		alertas[0].Dados["banco_centavos"] != float64(100000) || alertas[0].Dados["calculado_centavos"] != float64(97410) {
		t.Fatalf("alerta: %+v", alertas)
	}
	// sincronizar de novo no mesmo dia com a mesma diferença não repete o alerta
	sincronizar(t, amb, u)
	a.exigir("GET", "/api/alertas", nil, http.StatusOK).json(t, &alertas)
	if len(alertas) != 1 {
		t.Fatalf("alerta repetido: %d", len(alertas))
	}

	// acertar o saldo inicial fecha a diferença
	var aj struct {
		Ajuste int64 `json:"ajuste_centavos"`
	}
	a.exigir("POST", "/api/contas/"+nu+"/acertar-saldo", nil, http.StatusOK).json(t, &aj)
	if aj.Ajuste != 2590 {
		t.Fatalf("ajuste: %d", aj.Ajuste)
	}
	if rel := sincronizar(t, amb, u); rel.Divergencias != 0 {
		t.Fatalf("depois do acerto: %+v", rel)
	}

	// o banco muda sem transação nova (tarifa que não veio): nova diferença, novo alerta
	amb.pluggy.Saldo("acc", "990.00")
	if rel := sincronizar(t, amb, u); rel.Divergencias != 1 {
		t.Fatalf("tarifa sumida: %+v", rel)
	}
	a.exigir("GET", "/api/alertas", nil, http.StatusOK).json(t, &alertas)
	if len(alertas) != 2 || alertas[0].Dados["diferenca_centavos"] != float64(-1000) {
		t.Fatalf("segundo alerta: %+v", alertas)
	}
}

// OFX e Open Finance na mesma conta não duplicam: a mesma transação vira uma só.
func TestArquivoEOpenFinanceNaoDuplicam(t *testing.T) {
	amb, a, pf, nu, _ := prepara(t)
	arquivo := ofx(
		[4]string{dia(-5)[0:4] + dia(-5)[5:7] + dia(-5)[8:10], "-45.90", "fit1", "Padaria Pao Quente"},
		[4]string{dia(-3)[0:4] + dia(-3)[5:7] + dia(-3)[8:10], "-120.00", "fit2", "Farmacia"},
	)
	a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": nu, "arquivo": "a.ofx", "conteudo": arquivo}, http.StatusCreated)

	amb.pluggy.Cliente("cli", "sec")
	// 1000 − 45,90 − 120 − 40 (Uber, que só a Pluggy tem)
	amb.pluggy.Item("cli", "item", "Nubank", contaBanco("acc", "Nubank", "794.10",
		txPluggy("p1", -4, "-45.90", "DEBIT", "PADARIA PAO QUENTE LTDA"), // um dia depois do OFX: é a mesma
		txPluggy("p2", -3, "-120", "DEBIT", "DROGARIA SAO PAULO"),
		txPluggy("p3", -1, "-40", "DEBIT", "UBER"),
		pluggy.Transacao{ID: "p4", Date: diaPluggy(0), Amount: "-9.99", Type: "DEBIT", Status: "PENDING", Description: "PENDENTE"}))
	conectar(t, a, "cli", "sec", "item", pf)
	u := usuarioDe(t, a)
	sincronizar(t, amb, u)
	var e estadoOF
	a.exigir("GET", "/api/open-finance", nil, http.StatusOK).json(t, &e)
	a.exigir("PATCH", "/api/open-finance/contas/"+e.Itens[0].Contas[0].ID, map[string]string{"acao": "vincular", "conta_id": nu}, http.StatusNoContent)

	rel := sincronizar(t, amb, u)
	if rel.Novas != 1 || rel.Casadas != 2 || rel.Divergencias != 0 {
		t.Fatalf("sincronização: %+v", rel)
	}
	var l listaTransacoes
	a.exigir("GET", "/api/transacoes?conta_id="+nu, nil, http.StatusOK).json(t, &l)
	if l.Total != 3 {
		t.Fatalf("esperava 3 transações (2 do OFX casadas + Uber), veio %d", l.Total)
	}
	// reimportar o OFX e sincronizar de novo continua sem duplicar
	var r resultadoImp
	a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": nu, "arquivo": "a.ofx", "conteudo": arquivo}, http.StatusCreated).json(t, &r)
	if r.Novas != 0 {
		t.Fatalf("OFX de novo: %+v", r)
	}
	if rel := sincronizar(t, amb, u); rel.Novas != 0 {
		t.Fatalf("Pluggy de novo: %+v", rel)
	}
	// um OFX depois da Pluggy também casa (Uber veio primeiro pela Pluggy)
	depois := ofx([4]string{dia(-1)[0:4] + dia(-1)[5:7] + dia(-1)[8:10], "-40.00", "fit3", "Uber Trip"})
	a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": nu, "arquivo": "b.ofx", "conteudo": depois}, http.StatusCreated).json(t, &r)
	if r.Novas != 0 || r.Duplicadas != 1 {
		t.Fatalf("OFX depois da Pluggy: %+v", r)
	}
	a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": nu, "arquivo": "b.ofx", "conteudo": depois}, http.StatusCreated).json(t, &r)
	if r.Novas != 0 {
		t.Fatalf("mesmo OFX duas vezes depois da Pluggy: %+v", r)
	}
}

// Cartão pela Pluggy: vira conta de cartão com limite e dias, e a parcela entra
// com "(n/N)" para projetar as futuras.
func TestCartaoPeloOpenFinance(t *testing.T) {
	amb, a, pf, _, _ := prepara(t)
	amb.pluggy.Cliente("cli", "sec")
	compra := financeiro.Hoje().AddDate(0, -2, 0)
	parcela := pluggy.Transacao{ID: "c1", Date: compra.Format("2006-01-02"), Amount: "100", Type: "DEBIT", Status: "POSTED", Description: "LOJA DE MOVEIS"}
	parcela.CreditCardMetadata = &struct {
		InstallmentNumber int    `json:"installmentNumber"`
		TotalInstallments int    `json:"totalInstallments"`
		PurchaseDate      string `json:"purchaseDate"`
	}{3, 10, compra.Format("2006-01-02")}
	cartao := &pluggyfalsa.Conta{Conta: pluggy.Conta{ID: "card", Type: "CREDIT", Subtype: "CREDIT_CARD", Name: "Ultravioleta", Balance: "1234.56"},
		Transacoes: []pluggy.Transacao{parcela, txPluggy("c2", -1, "-500", "CREDIT", "PAGAMENTO RECEBIDO")}}
	cartao.Conta.CreditData = &struct {
		CreditLimit      json.Number `json:"creditLimit"`
		BalanceCloseDate string      `json:"balanceCloseDate"`
		BalanceDueDate   string      `json:"balanceDueDate"`
	}{"8000", "2026-10-03", "2026-10-10"}
	amb.pluggy.Item("cli", "item", "Nubank", cartao)
	conectar(t, a, "cli", "sec", "item", pf)
	u := usuarioDe(t, a)
	sincronizar(t, amb, u)
	var e estadoOF
	a.exigir("GET", "/api/open-finance", nil, http.StatusOK).json(t, &e)
	if *e.Itens[0].Contas[0].Saldo != -123456 {
		t.Fatalf("saldo do cartão é dívida: %+v", e.Itens[0].Contas[0])
	}
	a.exigir("PATCH", "/api/open-finance/contas/"+e.Itens[0].Contas[0].ID, map[string]string{"acao": "criar"}, http.StatusNoContent)
	if rel := sincronizar(t, amb, u); rel.Novas != 2 || rel.Divergencias != 0 {
		t.Fatalf("cartão: %+v", rel)
	}
	var contas []struct {
		ID         string `json:"id"`
		Tipo       string `json:"tipo"`
		Limite     *int64 `json:"limite_centavos"`
		Fechamento *int16 `json:"fechamento"`
		Vencimento *int16 `json:"vencimento"`
	}
	a.exigir("GET", "/api/contas", nil, http.StatusOK).json(t, &contas)
	var c string
	for _, x := range contas {
		if x.Tipo == "cartao" {
			c = x.ID
			if x.Limite == nil || *x.Limite != 800000 || *x.Fechamento != 3 || *x.Vencimento != 10 {
				t.Fatalf("cartão criado: %+v", x)
			}
		}
	}
	var l listaTransacoes
	a.exigir("GET", "/api/transacoes?conta_id="+c, nil, http.StatusOK).json(t, &l)
	achou := false
	for _, x := range l.Itens {
		if x.Descricao == "LOJA DE MOVEIS (3/10)" {
			achou = x.Data == compra.AddDate(0, 2, 0).Format("2006-01-02") && x.Valor == -10000
		}
		if x.Descricao == "PAGAMENTO RECEBIDO" && x.Valor != 50000 {
			t.Errorf("pagamento do cartão: %+v", x)
		}
	}
	if !achou {
		t.Fatalf("parcela: %+v", l.Itens)
	}
	var f faturasResp
	a.exigir("GET", "/api/contas/"+c+"/faturas", nil, http.StatusOK).json(t, &f)
	// a 3ª está na fatura aberta; da 4ª à 10ª são projetadas
	projetadas := 0
	for _, p := range f.Parcelas {
		if p.Projetada {
			projetadas++
		}
	}
	if len(f.Parcelas) != 8 || projetadas != 7 {
		t.Fatalf("parcelas futuras: %d, projetadas %d", len(f.Parcelas), projetadas)
	}
}

// Worker, banco com erro e /api/health.
func TestSincronizacaoDiariaEHealth(t *testing.T) {
	amb, a, pf, _, _ := prepara(t)
	if err := worker.GravarSinal(context.Background(), amb.banco.App, "worker", nil); err != nil {
		t.Fatal(err)
	}
	var h map[string]any
	amb.novoCliente().exigir("GET", "/api/health", nil, http.StatusOK).json(t, &h)
	if h["pluggy"] != "ok" || h["last_sync"] != nil {
		t.Fatalf("health sem conexões: %v", h)
	}

	amb.pluggy.Cliente("cli", "sec")
	amb.pluggy.Item("cli", "item", "Inter", contaBanco("acc", "Inter", "10"))
	conectar(t, a, "cli", "sec", "item", pf)
	// o worker pega quem nunca sincronizou
	if err := worker.SincronizarAtrasados(context.Background(), amb.banco.App, amb.of, time.Now().Add(-20*time.Hour)); err != nil {
		t.Fatal(err)
	}
	amb.novoCliente().exigir("GET", "/api/health", nil, http.StatusOK).json(t, &h)
	if h["pluggy"] != "ok" || h["last_sync"] == nil {
		t.Fatalf("health depois da sincronização: %v", h)
	}
	chamadas := amb.pluggy.Chamadas
	// e não sincroniza de novo quem está em dia
	if err := worker.SincronizarAtrasados(context.Background(), amb.banco.App, amb.of, time.Now().Add(-20*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if amb.pluggy.Chamadas != chamadas {
		t.Fatal("sincronizou quem já estava em dia")
	}

	// banco pedindo login de novo: o erro aparece na tela e no health (sem derrubar o 200)
	amb.pluggy.Status("item", "LOGIN_ERROR")
	sincronizar(t, amb, usuarioDe(t, a))
	var e estadoOF
	a.exigir("GET", "/api/open-finance", nil, http.StatusOK).json(t, &e)
	if e.Itens[0].UltimoErro == nil {
		t.Fatalf("erro do item: %+v", e.Itens[0])
	}
	amb.novoCliente().exigir("GET", "/api/health", nil, http.StatusOK).json(t, &h)
	if h["pluggy"] != "erro" {
		t.Fatalf("health com banco em erro: %v", h)
	}

	// credencial revogada no Meu Pluggy: fica o erro na conexão
	amb.pluggy.Cliente("cli", "outra")
	if _, err := amb.of.Sincronizar(context.Background(), usuarioDe(t, a)); err == nil {
		t.Fatal("credencial revogada deveria falhar")
	}
	a.exigir("GET", "/api/open-finance", nil, http.StatusOK).json(t, &e)
	if e.Conexao.UltimoErro == nil {
		t.Fatal("erro da conexão não gravado")
	}
}
