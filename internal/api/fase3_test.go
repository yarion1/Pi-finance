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
	cartao.Conta.CreditData = &pluggy.DadosCartao{CreditLimit: "8000", AvailableCreditLimit: "6500.50", BalanceCloseDate: "2026-10-03", BalanceDueDate: "2026-10-10"}
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
	var contas []cartaoListado
	a.exigir("GET", "/api/contas", nil, http.StatusOK).json(t, &contas)
	var c string
	for _, x := range contas {
		if x.Tipo == "cartao" {
			c = x.ID
			// limite usado segundo o banco: 8.000 − 6.500,50 disponível
			if x.Limite == nil || *x.Limite != 800000 || *x.Fechamento != 3 || *x.Vencimento != 10 ||
				x.UsadoBanco == nil || *x.UsadoBanco != 149950 {
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

type cartaoListado struct {
	ID         string `json:"id"`
	Nome       string `json:"nome"`
	Tipo       string `json:"tipo"`
	Limite     *int64 `json:"limite_centavos"`
	Fechamento *int16 `json:"fechamento"`
	Vencimento *int16 `json:"vencimento"`
	UsadoBanco *int64 `json:"limite_usado_banco_centavos"`
}

// Cartão ligado a uma conta que já existia sem dias nem limite: o banco preenche, e as
// compras já importadas ganham fatura.
func TestCartaoLigadoRecebeLimiteEDias(t *testing.T) {
	amb, a, pf, _, _ := prepara(t)
	var existente struct{ ID string }
	a.exigir("POST", "/api/contas", map[string]any{"entidade_id": pf, "nome": "Meu cartão", "tipo": "cartao"}, http.StatusCreated).json(t, &existente)
	amb.pluggy.Cliente("cli", "sec")
	cartao := &pluggyfalsa.Conta{Conta: pluggy.Conta{ID: "card", Type: "CREDIT", Name: "Gold", Balance: "300",
		CreditData: &pluggy.DadosCartao{CreditLimit: "5000", AvailableCreditLimit: "4700", BalanceCloseDate: "2026-10-05", BalanceDueDate: "2026-10-12"}},
		Transacoes: []pluggy.Transacao{txPluggy("c1", -3, "300", "DEBIT", "MERCADO")}}
	amb.pluggy.Item("cli", "item", "Nubank", cartao)
	conectar(t, a, "cli", "sec", "item", pf)
	u := usuarioDe(t, a)
	sincronizar(t, amb, u)
	var e estadoOF
	a.exigir("GET", "/api/open-finance", nil, http.StatusOK).json(t, &e)
	a.exigir("PATCH", "/api/open-finance/contas/"+e.Itens[0].Contas[0].ID, map[string]string{"acao": "vincular", "conta_id": existente.ID}, http.StatusNoContent)
	if rel := sincronizar(t, amb, u); rel.Novas != 1 {
		t.Fatalf("sincronização: %+v", rel)
	}
	var contas []cartaoListado
	a.exigir("GET", "/api/contas", nil, http.StatusOK).json(t, &contas)
	for _, x := range contas {
		if x.ID == existente.ID && (x.Limite == nil || *x.Limite != 500000 || x.Fechamento == nil || *x.Fechamento != 5 ||
			*x.Vencimento != 12 || x.UsadoBanco == nil || *x.UsadoBanco != 30000) {
			t.Fatalf("cartão ligado: %+v", x)
		}
	}
	var f faturasResp
	a.exigir("GET", "/api/contas/"+existente.ID+"/faturas", nil, http.StatusOK).json(t, &f)
	total := int64(0)
	for _, x := range f.Faturas {
		total += x.Compras
	}
	if total != 30000 {
		t.Fatalf("compra sem fatura: %+v", f.Faturas)
	}
}

// A categoria que o banco dá entra quando não há regra nem histórico, inclusive nas
// transações que já estavam no painel sem categoria; pagamento de fatura vira
// transferência (não é gasto).
func TestCategoriaDoBanco(t *testing.T) {
	amb, a, pf, nu, _ := prepara(t)
	mercado := txPluggy("p1", -3, "-80", "DEBIT", "SUPERMERCADO DIA")
	mercado.Category, mercado.CategoryID = "Groceries", "10000000"
	uber := txPluggy("p2", -2, "-25", "DEBIT", "UBER *TRIP")
	pix := txPluggy("p3", -1, "-50", "DEBIT", "PIX ENVIADO FULANO")
	pix.Category, pix.CategoryID = "Transfer - PIX", "05070000"
	fatura := txPluggy("p4", -1, "-300", "DEBIT", "PAGAMENTO FATURA")
	fatura.Category, fatura.CategoryID = "Credit card payment", "05100000"
	amb.pluggy.Cliente("cli", "sec")
	amb.pluggy.Item("cli", "item", "Nubank", contaBanco("acc", "Nubank", "545", mercado, uber, pix, fatura))
	conectar(t, a, "cli", "sec", "item", pf)
	u := usuarioDe(t, a)
	sincronizar(t, amb, u)
	var e estadoOF
	a.exigir("GET", "/api/open-finance", nil, http.StatusOK).json(t, &e)
	a.exigir("PATCH", "/api/open-finance/contas/"+e.Itens[0].Contas[0].ID, map[string]string{"acao": "vincular", "conta_id": nu}, http.StatusNoContent)
	sincronizar(t, amb, u)

	var l listaTransacoes
	a.exigir("GET", "/api/transacoes?conta_id="+nu, nil, http.StatusOK).json(t, &l)
	cat := map[string]string{}
	tipo := map[string]string{}
	for _, x := range l.Itens {
		if x.Categoria != nil {
			cat[x.Descricao] = *x.Categoria
		}
		tipo[x.Descricao] = x.Tipo
	}
	if cat["SUPERMERCADO DIA"] != "Mercado" || cat["PAGAMENTO FATURA"] != "Pagamento de fatura" ||
		tipo["PAGAMENTO FATURA"] != "transferencia" || cat["PIX ENVIADO FULANO"] != "" || cat["UBER *TRIP"] != "" {
		t.Fatalf("categorias: %v tipos: %v", cat, tipo)
	}

	// a Uber já estava no painel sem categoria; o banco passa a categorizar e a próxima
	// sincronização (que relê o período) completa
	amb.pluggy.Categoria("acc", "p2", "19010000", "Taxi and ride-hailing")
	sincronizar(t, amb, u)
	a.exigir("GET", "/api/transacoes?conta_id="+nu+"&busca=UBER", nil, http.StatusOK).json(t, &l)
	if len(l.Itens) != 1 || l.Itens[0].Categoria == nil || *l.Itens[0].Categoria != "App" {
		t.Fatalf("uber completada: %+v", l.Itens)
	}
}

// Investimentos do banco viram ativos com o saldo informado; ativo manual vale a posição
// pelas operações × a última cotação; tudo entra no patrimônio.
func TestInvestimentosDoBanco(t *testing.T) {
	amb, a, pf, _, _ := prepara(t)
	amb.pluggy.Cliente("cli", "sec")
	amb.pluggy.Item("cli", "item", "MeuPluggy")
	amb.pluggy.Investimentos("item",
		pluggy.Investimento{ID: "i1", Name: "CDB - PICPAY", Type: "FIXED_INCOME", Subtype: "CDB", Balance: "12726.64",
			AmountOriginal: "12000", Issuer: "PICPAY INSTITUICAO DE PAGAMENTO S/A", Status: "ACTIVE", DueDate: "2028-01-10"},
		pluggy.Investimento{ID: "i2", Name: "LCI BANCO X", Type: "FIXED_INCOME", Subtype: "LCI", Balance: "5000", Status: "ACTIVE"},
		pluggy.Investimento{ID: "i3", Name: "CDB - PICPAY", Type: "FIXED_INCOME", Subtype: "CDB", Balance: "0", Status: "TOTAL_WITHDRAWAL"},
		pluggy.Investimento{ID: "i4", Name: "Tesouro Selic 2029", Type: "FIXED_INCOME", Subtype: "TREASURY", Balance: "1000", Status: "ACTIVE"},
	)
	conectar(t, a, "cli", "sec", "item", pf)
	sincronizar(t, amb, usuarioDe(t, a))
	// de novo: atualiza sem duplicar
	amb.pluggy.Investimentos("item",
		pluggy.Investimento{ID: "i1", Name: "CDB - PICPAY", Type: "FIXED_INCOME", Subtype: "CDB", Balance: "12800",
			AmountOriginal: "12000", Issuer: "PICPAY INSTITUICAO DE PAGAMENTO S/A", Status: "ACTIVE"},
		pluggy.Investimento{ID: "i2", Name: "LCI BANCO X", Type: "FIXED_INCOME", Subtype: "LCI", Balance: "5000", Status: "ACTIVE"},
		pluggy.Investimento{ID: "i3", Name: "CDB - PICPAY", Type: "FIXED_INCOME", Subtype: "CDB", Balance: "0", Status: "TOTAL_WITHDRAWAL"},
		pluggy.Investimento{ID: "i4", Name: "Tesouro Selic 2029", Type: "FIXED_INCOME", Subtype: "TREASURY", Balance: "1000", Status: "ACTIVE"},
	)
	sincronizar(t, amb, usuarioDe(t, a))

	// ativo manual com uma compra e a cotação gravada pelo worker
	ctx := context.Background()
	var ativo string
	if err := amb.banco.Dono.QueryRow(ctx, `insert into ativos (entidade_id, codigo, classe, cotacao_codigo) values ($1, 'PETR4', 'acao', 'PETR4') returning id`, pf).Scan(&ativo); err != nil {
		t.Fatal(err)
	}
	if _, err := amb.banco.Dono.Exec(ctx, `insert into operacoes (entidade_id, ativo_id, data, tipo, quantidade, preco) values ($1, $2, '2026-01-05', 'compra', 100, 30)`, pf, ativo); err != nil {
		t.Fatal(err)
	}
	if _, err := amb.banco.App.Exec(ctx, "insert into cotacoes (codigo, data, preco, fonte) values ('PETR4', '2026-06-01', 38.5, 'teste')"); err != nil {
		t.Fatal(err)
	}

	var c struct {
		Total      int64 `json:"total_centavos"`
		Encerrados int   `json:"encerrados"`
		Classes    []struct {
			Classe string `json:"classe"`
			Valor  int64  `json:"valor_centavos"`
			Ativos int    `json:"ativos"`
		} `json:"classes"`
		Ativos []struct {
			Nome        string  `json:"nome"`
			Instituicao *string `json:"instituicao"`
			Valor       int64   `json:"valor_centavos"`
			Rendimento  int64   `json:"rendimento_centavos"`
			Isento      bool    `json:"isento_ir"`
			Quantidade  string  `json:"quantidade"`
		} `json:"ativos"`
	}
	a.exigir("GET", "/api/investimentos", nil, http.StatusOK).json(t, &c)
	// 12.800 + 5.000 + 1.000 + 100 × 38,50
	if c.Total != 1280000+500000+100000+385000 || c.Encerrados != 1 || len(c.Classes) != 3 {
		t.Fatalf("carteira: %+v", c)
	}
	if c.Ativos[0].Nome != "CDB - PICPAY" || c.Ativos[0].Rendimento != 80000 || c.Ativos[0].Instituicao == nil ||
		*c.Ativos[0].Instituicao != "PICPAY INSTITUICAO DE PAGAMENTO S/A" {
		t.Fatalf("CDB: %+v", c.Ativos[0])
	}
	for _, x := range c.Ativos {
		if x.Nome == "LCI BANCO X" && !x.Isento {
			t.Error("LCI é isenta")
		}
		if x.Nome == "PETR4" && (x.Valor != 385000 || x.Quantidade != "100") {
			t.Errorf("PETR4: %+v", x)
		}
	}
	var p struct {
		Investimentos int64 `json:"investimentos_centavos"`
	}
	a.exigir("GET", "/api/patrimonio", nil, http.StatusOK).json(t, &p)
	if p.Investimentos != c.Total {
		t.Fatalf("patrimônio sem a carteira: %d", p.Investimentos)
	}
}

// Extras do Meu Pluggy: faturas do cartão, empréstimos, identidade e movimentações dos
// investimentos, cada um opcional.
func TestExtrasDoBanco(t *testing.T) {
	amb, a, pf, _, _ := prepara(t)
	amb.pluggy.Cliente("cli", "sec")
	cartao := &pluggyfalsa.Conta{Conta: pluggy.Conta{ID: "card", Type: "CREDIT", Subtype: "CREDIT_CARD", Name: "Ultravioleta", Balance: "900"},
		Faturas: []pluggy.Fatura{
			{ID: "b1", DueDate: dia(-20) + "T00:00:00.000Z", TotalAmount: "1234.56", MinimumPayment: "185.18", CurrencyCode: "BRL"},
			{ID: "b2", DueDate: dia(10) + "T00:00:00.000Z", TotalAmount: "900", CurrencyCode: "BRL",
				FinanceCharges: []pluggy.EncargoFatura{{Type: "IOF", Amount: "3.5"}, {Type: "LATE_PAYMENT_FEE", Amount: "10"}}},
		}}
	amb.pluggy.Item("cli", "item", "Nubank", contaBanco("conta", "Conta", "1000"), cartao)
	amb.pluggy.Investimentos("item", pluggy.Investimento{ID: "inv-1", Name: "CDB X", Type: "FIXED_INCOME", Subtype: "CDB",
		Balance: "12726.64", AmountOriginal: "12000", Status: "ACTIVE"})
	cinco, doze, tres := 5, 12, 7
	credito := "CREDIT"
	amb.pluggy.Extras("item",
		[]pluggy.Emprestimo{
			{ID: "l1", ProductName: "Crédito pessoal", Kind: "LOAN", ContractAmount: "10000", CurrencyCode: "BRL", CET: "0.035",
				Installments: &pluggy.ParcelasEmprestimo{TotalNumberOfInstallments: &doze, PaidInstallments: &cinco, DueInstallments: &tres},
				Payments:     &pluggy.PagamentosEmprestimo{ContractOutstandingBalance: "6543.21"}},
			{ID: "l2", ProductName: "Cheque especial", Kind: "UNARRANGED_ACCOUNT_OVERDRAFT", CurrencyCode: "BRL",
				Payments: &pluggy.PagamentosEmprestimo{ContractOutstandingBalance: "300"}},
		},
		&pluggy.Identidade{FullName: strPtr("Ana Teste"), Document: strPtr("529.982.247-25"), DocumentType: strPtr("CPF")},
		map[string][]pluggy.MovimentoInvestimento{"inv-1": {
			{ID: "m1", MovementType: credito, Type: strPtr("BUY"), Amount: "12000", Date: dia(-400)},
			{ID: "m2", MovementType: "DEBIT", Type: strPtr("TAX"), Amount: "10", Date: dia(-10)},
		}})
	conectar(t, a, "cli", "sec", "item", pf)
	u := usuarioDe(t, a)
	sincronizar(t, amb, u)
	var e estadoOF
	a.exigir("GET", "/api/open-finance", nil, http.StatusOK).json(t, &e)
	for _, c := range e.Itens[0].Contas {
		a.exigir("PATCH", "/api/open-finance/contas/"+c.ID, map[string]string{"acao": "criar"}, http.StatusNoContent)
	}
	if rel := sincronizar(t, amb, u); len(rel.Erros) != 0 {
		t.Fatalf("erros: %+v", rel.Erros)
	}
	sincronizar(t, amb, u) // de novo: nada duplica

	// identidade: o CPF da PF vazia é preenchido (cifrado, mostrado mascarado)
	var ent struct{ Documento *string }
	a.exigir("GET", "/api/entidades/"+pf, nil, http.StatusOK).json(t, &ent)
	if ent.Documento == nil || *ent.Documento != "***.982.247-**" {
		t.Fatalf("CPF da identidade: %v", ent.Documento)
	}

	// faturas do banco no cartão
	var contas []struct {
		ID, Nome, Tipo string
	}
	a.exigir("GET", "/api/contas", nil, http.StatusOK).json(t, &contas)
	var cartaoID string
	for _, c := range contas {
		if c.Tipo == "cartao" {
			cartaoID = c.ID
		}
	}
	var fat struct {
		FaturasBanco []struct {
			Vencimento string `json:"vencimento"`
			Total      int64  `json:"total_centavos"`
			Minimo     *int64 `json:"minimo_centavos"`
			Encargos   int64  `json:"encargos_centavos"`
		} `json:"faturas_banco"`
	}
	a.exigir("GET", "/api/contas/"+cartaoID+"/faturas", nil, http.StatusOK).json(t, &fat)
	if len(fat.FaturasBanco) != 2 || fat.FaturasBanco[0].Total != 90000 || fat.FaturasBanco[0].Encargos != 1350 ||
		fat.FaturasBanco[1].Minimo == nil || *fat.FaturasBanco[1].Minimo != 18518 {
		t.Fatalf("faturas do banco: %+v", fat.FaturasBanco)
	}

	// empréstimos: o crédito pessoal entra no passivo; o cheque especial já está no saldo da conta
	var pat struct {
		DividasBanco int64 `json:"dividas_banco_centavos"`
		Emprestimos  []struct {
			Nome           string `json:"nome"`
			Saldo          *int64 `json:"saldo_devedor_centavos"`
			Restantes      *int   `json:"parcelas_restantes"`
			ContaNoPassivo bool   `json:"conta_no_passivo"`
		} `json:"emprestimos_banco"`
	}
	a.exigir("GET", "/api/patrimonio", nil, http.StatusOK).json(t, &pat)
	if pat.DividasBanco != 654321 || len(pat.Emprestimos) != 2 || !pat.Emprestimos[0].ContaNoPassivo ||
		*pat.Emprestimos[0].Restantes != 7 || pat.Emprestimos[1].ContaNoPassivo {
		t.Fatalf("empréstimos: %+v", pat)
	}

	// movimentos: o aporte de 400 dias atrás dá a rentabilidade ao ano do CDB (o imposto fica de fora)
	var cart struct {
		Ativos []struct {
			Nome string   `json:"nome"`
			Rent *float64 `json:"rentabilidade_aa"`
		} `json:"ativos"`
	}
	a.exigir("GET", "/api/investimentos", nil, http.StatusOK).json(t, &cart)
	if len(cart.Ativos) != 1 || cart.Ativos[0].Rent == nil || *cart.Ativos[0].Rent < 0.05 || *cart.Ativos[0].Rent > 0.06 {
		t.Fatalf("rentabilidade do CDB do banco: %+v", cart.Ativos)
	}
	var n int
	_ = amb.banco.Dono.QueryRow(context.Background(), "select count(*) from operacoes where origem = 'pluggy'").Scan(&n)
	if n != 1 {
		t.Fatalf("operações do banco: %d", n)
	}

	// empréstimo quitado some da Pluggy e daqui; banco sem os extras não quebra a sincronização
	amb.pluggy.Extras("item", nil, nil, nil)
	if rel := sincronizar(t, amb, u); len(rel.Erros) != 0 {
		t.Fatalf("sem extras: %+v", rel.Erros)
	}
	a.exigir("GET", "/api/patrimonio", nil, http.StatusOK).json(t, &pat)
	if len(pat.Emprestimos) != 2 {
		t.Fatalf("sem /loans (404) mantém o que havia: %+v", pat.Emprestimos)
	}
	amb.pluggy.Extras("item", []pluggy.Emprestimo{}, nil, nil)
	sincronizar(t, amb, u)
	a.exigir("GET", "/api/patrimonio", nil, http.StatusOK).json(t, &pat)
	if len(pat.Emprestimos) != 0 || pat.DividasBanco != 0 {
		t.Fatalf("quitados: %+v", pat)
	}
}

func strPtr(s string) *string { return &s }
