package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/pquerna/otp/totp"

	"github.com/yarion1/pi-finance/internal/api"
	"github.com/yarion1/pi-finance/internal/auth"
	"github.com/yarion1/pi-finance/internal/config"
	"github.com/yarion1/pi-finance/internal/cripto"
	"github.com/yarion1/pi-finance/internal/db/dbteste"
	"github.com/yarion1/pi-finance/internal/openfinance"
	"github.com/yarion1/pi-finance/internal/pluggy"
	"github.com/yarion1/pi-finance/internal/pluggy/pluggyfalsa"
	"github.com/yarion1/pi-finance/internal/worker"
)

const origemTeste = "https://pi.teste.ts.net:8443"

type ambiente struct {
	t       *testing.T
	handler http.Handler
	banco   *dbteste.Banco
	pluggy  *pluggyfalsa.Falsa   // a API da Pluggy de mentira que o servidor usa
	of      *openfinance.Servico // o mesmo serviço do servidor (o worker usa igual)
}

func novoAmbiente(t *testing.T, ajustar ...func(*config.Config)) *ambiente {
	t.Helper()
	banco := dbteste.Novo(t)
	cif, _ := cripto.Novo(bytes.Repeat([]byte{9}, 32))
	wa, err := auth.NovoWebAuthn(origemTeste)
	if err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{URLPublica: origemTeste, HSTS: true}
	for _, f := range ajustar {
		f(&cfg)
	}
	falsa := pluggyfalsa.Nova()
	srvPluggy := httptest.NewServer(falsa)
	t.Cleanup(srvPluggy.Close)
	of := &openfinance.Servico{Banco: banco.App, Cifrador: cif, Pluggy: pluggy.Novo(srvPluggy.URL)}
	srv := &api.Servidor{
		Auth: &auth.Servico{Pool: banco.App, Cifrador: cif, WebAuthn: wa},
		Pool: banco.App, Cifrador: cif, Config: cfg, Versao: "v0.0.0-teste",
		Front:       fstest.MapFS{"index.html": {Data: []byte("<!doctype html><title>Finanças</title>")}},
		OpenFinance: of,
	}
	return &ambiente{t: t, handler: srv.Handler(), banco: banco, pluggy: falsa, of: of}
}

// cliente guarda os cookies como um navegador faria.
type cliente struct {
	amb     *ambiente
	cookies map[string]string
	origem  string
	ua      string
}

func (a *ambiente) novoCliente() *cliente {
	return &cliente{amb: a, cookies: map[string]string{}, origem: origemTeste, ua: "Mozilla/5.0 (X11; Linux) Firefox/140.0"}
}

type resposta struct {
	status int
	corpo  []byte
	cab    http.Header
}

func (r resposta) json(t *testing.T, v any) {
	t.Helper()
	if err := json.Unmarshal(r.corpo, v); err != nil {
		t.Fatalf("json: %v (%s)", err, r.corpo)
	}
}

func (c *cliente) fazer(metodo, caminho string, corpo any) resposta {
	c.amb.t.Helper()
	var leitor io.Reader
	if corpo != nil {
		b, _ := json.Marshal(corpo)
		leitor = bytes.NewReader(b)
	}
	req := httptest.NewRequest(metodo, caminho, leitor)
	if corpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.origem != "" {
		req.Header.Set("Origin", c.origem)
	}
	req.Header.Set("User-Agent", c.ua)
	for k, v := range c.cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	rec := httptest.NewRecorder()
	c.amb.handler.ServeHTTP(rec, req)
	for _, ck := range rec.Result().Cookies() {
		if ck.MaxAge < 0 {
			delete(c.cookies, ck.Name)
		} else {
			c.cookies[ck.Name] = ck.Value
		}
	}
	return resposta{status: rec.Code, corpo: rec.Body.Bytes(), cab: rec.Header()}
}

func (c *cliente) exigir(metodo, caminho string, corpo any, status int) resposta {
	c.amb.t.Helper()
	r := c.fazer(metodo, caminho, corpo)
	if r.status != status {
		c.amb.t.Fatalf("%s %s: status %d, esperado %d: %s", metodo, caminho, r.status, status, r.corpo)
	}
	return r
}

// cadastrarCom2FA cadastra, ativa o TOTP e deixa a sessão completa.
func (c *cliente) cadastrarCom2FA(nome, email, convite string) {
	c.amb.t.Helper()
	c.exigir("POST", "/api/auth/cadastro", map[string]string{
		"nome": nome, "email": email, "senha": "senha comprida de teste", "convite": convite,
	}, http.StatusCreated)
	var ini struct{ Segredo string }
	c.exigir("POST", "/api/auth/totp/iniciar", nil, http.StatusOK).json(c.amb.t, &ini)
	codigo, _ := totp.GenerateCode(ini.Segredo, time.Now())
	var conf struct {
		Codigos []string `json:"codigos_recuperacao"`
	}
	c.exigir("POST", "/api/auth/totp/confirmar", map[string]string{"codigo": codigo}, http.StatusOK).json(c.amb.t, &conf)
	if len(conf.Codigos) != 10 {
		c.amb.t.Fatalf("códigos de recuperação: %v", conf.Codigos)
	}
}

func TestIsolamentoPorRota(t *testing.T) {
	amb := novoAmbiente(t)
	a := amb.novoCliente()
	a.cadastrarCom2FA("Ana", "ana@teste.com", "")

	var casa struct{ ID string }
	a.exigir("POST", "/api/casas", map[string]string{"nome": "Casa"}, http.StatusCreated).json(t, &casa)
	var pf struct{ ID string }
	a.exigir("POST", "/api/entidades", map[string]any{"tipo": "PF", "nome": "Ana PF", "documento": "529.982.247-25"}, http.StatusCreated).json(t, &pf)
	var convite struct{ Link string }
	a.exigir("POST", "/api/casas/"+casa.ID+"/convites", map[string]string{"papel": "membro"}, http.StatusCreated).json(t, &convite)
	token := convite.Link[strings.LastIndex(convite.Link, "/")+1:]

	// dados privados de A, criados pelas próprias rotas
	var conta struct{ ID string }
	a.exigir("POST", "/api/contas", map[string]any{"entidade_id": pf.ID, "nome": "CONTA-SECRETA", "tipo": "corrente"}, http.StatusCreated).json(t, &conta)
	contaPrivada := conta.ID
	var imp struct {
		ImportacaoID string `json:"importacao_id"`
	}
	a.exigir("POST", "/api/importacoes", map[string]any{
		"conta_id": contaPrivada, "arquivo": "ARQUIVO-SECRETO.ofx", "conteudo": ofxBase64("SEGREDO-DA-ANA", "-999.99", "fit-1"),
	}, http.StatusCreated).json(t, &imp)
	var cat struct{ ID string }
	a.exigir("POST", "/api/categorias", map[string]any{"entidade_id": pf.ID, "nome": "CATEGORIA-SECRETA", "tipo": "gasto"}, http.StatusCreated).json(t, &cat)
	a.exigir("POST", "/api/regras", map[string]any{"entidade_id": pf.ID, "texto": "REGRA-SECRETA", "categoria_id": cat.ID}, http.StatusCreated)
	a.exigir("POST", "/api/mapeamentos", map[string]any{"nome": "MAPA-SECRETO", "config": map[string]string{"coluna_data": "x"}}, http.StatusCreated)
	// fase 2: cartão com compra parcelada, meta, bem, dívida, compromisso, recorrência e orçamento
	var cartao, divida struct{ ID string }
	a.exigir("POST", "/api/contas", map[string]any{"entidade_id": pf.ID, "nome": "CARTAO-SECRETO", "tipo": "cartao", "fechamento": 5, "vencimento": 12}, http.StatusCreated).json(t, &cartao)
	a.exigir("POST", "/api/transacoes", map[string]any{"conta_id": cartao.ID, "data": time.Now().Format("2006-01-02"), "descricao": "COMPRA-SECRETA", "valor_centavos": -120000, "parcelas": 12}, http.StatusCreated)
	a.exigir("POST", "/api/metas", map[string]any{"entidade_id": pf.ID, "nome": "META-SECRETA", "tipo": "viagem", "alvo_centavos": 500000, "contas_vinculadas": []string{contaPrivada}}, http.StatusCreated)
	a.exigir("POST", "/api/bens", map[string]any{"entidade_id": pf.ID, "nome": "BEM-SECRETO", "tipo": "imovel", "valor_centavos": 50000000}, http.StatusCreated)
	a.exigir("POST", "/api/dividas", map[string]any{"entidade_id": pf.ID, "nome": "DIVIDA-SECRETA", "tipo": "financiamento", "sistema": "price",
		"principal_centavos": 10000000, "taxa_mensal": "0.01", "prazo_meses": 12, "primeiro_vencimento": time.Now().AddDate(0, 0, 10).Format("2006-01-02")}, http.StatusCreated).json(t, &divida)
	a.exigir("POST", "/api/compromissos", map[string]any{"entidade_id": pf.ID, "descricao": "COMPROMISSO-SECRETO", "valor_centavos": -30000, "vencimento": time.Now().AddDate(0, 0, 3).Format("2006-01-02")}, http.StatusCreated)
	a.exigir("POST", "/api/recorrencias", map[string]any{"entidade_id": pf.ID, "descricao": "RECORRENCIA-SECRETA", "valor_centavos": -3990, "frequencia": "mensal", "proxima": time.Now().AddDate(0, 0, 5).Format("2006-01-02"), "tipo": "assinatura"}, http.StatusCreated)
	a.exigir("PUT", "/api/orcamento", map[string]any{"entidade_id": pf.ID, "mes": time.Now().Format("2006-01"), "itens": []map[string]any{{"categoria_id": cat.ID, "limite_centavos": 7777777}}}, http.StatusNoContent)
	// Open Finance de A: credenciais, banco e conta da Pluggy
	amb.pluggy.Cliente("CLIENTE-SECRETO-QRST", "SECRET-SECRETO")
	amb.pluggy.Item("CLIENTE-SECRETO-QRST", "ITEM-SECRETO", "BANCO-SECRETO",
		contaBanco("acc-secreta", "CONTA-PLUGGY-SECRETA", "98765.43", txPluggy("tp", -1, "-1", "DEBIT", "PLUGGY-SECRETO")))
	conectar(t, a, "CLIENTE-SECRETO-QRST", "SECRET-SECRETO", "ITEM-SECRETO", pf.ID)
	sincronizar(t, amb, usuarioDe(t, a))
	// A enxerga o próprio segredo pelas rotas (controle positivo)
	if r := a.exigir("GET", "/api/transacoes", nil, 200); !bytes.Contains(r.corpo, []byte("SEGREDO-DA-ANA")) {
		t.Fatal("A deveria ver a própria transação")
	}

	b := amb.novoCliente()
	b.cadastrarCom2FA("Bruno", "bruno@teste.com", token)

	proibidos := []string{"SEGREDO-DA-ANA", "CONTA-SECRETA", contaPrivada, pf.ID, "982.247", "52998224725",
		"ARQUIVO-SECRETO", imp.ImportacaoID, "CATEGORIA-SECRETA", "REGRA-SECRETA", "MAPA-SECRETO",
		"CARTAO-SECRETO", cartao.ID, "COMPRA-SECRETA", "META-SECRETA", "BEM-SECRETO", "DIVIDA-SECRETA", divida.ID,
		"COMPROMISSO-SECRETO", "RECORRENCIA-SECRETA", "7777777", "50000000",
		"ITEM-SECRETO", "BANCO-SECRETO", "CONTA-PLUGGY-SECRETA", "9876543", "QRST", "SECRET-SECRETO"}
	for _, rota := range api.RotasLeitura {
		caminho := strings.NewReplacer("{casa}", casa.ID, "{entidade}", pf.ID, "{conta}", contaPrivada,
			"{importacao}", imp.ImportacaoID, "{cartao}", cartao.ID, "{divida}", divida.ID).Replace(rota)
		r := b.fazer("GET", caminho, nil)
		if r.status >= 500 {
			t.Errorf("%s: status %d", caminho, r.status)
		}
		for _, p := range proibidos {
			if bytes.Contains(r.corpo, []byte(p)) {
				t.Errorf("B leu %q em %s: %s", p, caminho, r.corpo)
			}
		}
		// sem login, nada
		if r := amb.novoCliente().fazer("GET", caminho, nil); r.status != http.StatusUnauthorized && caminho != "/api/auth/sessao" {
			t.Errorf("%s sem sessão: status %d", caminho, r.status)
		}
	}

	// B também não altera nada de A
	b.exigir("PATCH", "/api/entidades/"+pf.ID, map[string]any{"tipo": "PF", "nome": "hack"}, http.StatusNotFound)
	b.exigir("DELETE", "/api/entidades/"+pf.ID, nil, http.StatusForbidden) // pede reautenticação antes
	b.exigir("POST", "/api/entidades/"+pf.ID+"/acessos", map[string]string{"email": "bruno@teste.com", "papel": "membro"}, http.StatusForbidden)
	b.exigir("POST", "/api/casas/"+casa.ID+"/convites", map[string]string{"papel": "membro"}, http.StatusForbidden)
	b.exigir("POST", "/api/importacoes", map[string]any{"conta_id": contaPrivada, "arquivo": "x.ofx", "conteudo": ofxBase64("x", "-1.00", "z")}, http.StatusNotFound)
	b.exigir("DELETE", "/api/importacoes/"+imp.ImportacaoID, nil, http.StatusNotFound)
	b.exigir("POST", "/api/transacoes", map[string]any{"conta_id": contaPrivada, "data": "2026-09-01", "descricao": "x", "valor_centavos": -100}, http.StatusNotFound)
	b.exigir("POST", "/api/categorias", map[string]any{"entidade_id": pf.ID, "nome": "x", "tipo": "gasto"}, http.StatusForbidden)
	b.exigir("POST", "/api/contas", map[string]any{"entidade_id": pf.ID, "nome": "x", "tipo": "corrente"}, http.StatusForbidden)
	b.exigir("PATCH", "/api/contas/"+contaPrivada, map[string]any{"nome": "x", "tipo": "corrente"}, http.StatusNotFound)
	b.exigir("GET", "/api/resumo?entidade_id="+pf.ID, nil, http.StatusNotFound)
	b.exigir("POST", "/api/metas", map[string]any{"entidade_id": pf.ID, "nome": "x", "tipo": "outra", "alvo_centavos": 1}, http.StatusForbidden)
	b.exigir("PATCH", "/api/dividas/"+divida.ID, map[string]any{"entidade_id": pf.ID, "nome": "x", "tipo": "outro", "sistema": "sac",
		"principal_centavos": 1, "taxa_mensal": "0", "prazo_meses": 1, "primeiro_vencimento": "2026-01-01"}, http.StatusNotFound)
	b.exigir("DELETE", "/api/dividas/"+divida.ID, nil, http.StatusNotFound)
	b.exigir("PUT", "/api/orcamento", map[string]any{"entidade_id": pf.ID, "mes": "2026-09", "itens": []map[string]any{{"categoria_id": cat.ID, "limite_centavos": 1}}}, http.StatusForbidden)
	b.exigir("POST", "/api/recorrencias/detectar", map[string]any{"entidade_id": pf.ID}, http.StatusNotFound)
	if r := a.exigir("GET", "/api/transacoes?busca=SEGREDO", nil, 200); !bytes.Contains(r.corpo, []byte("SEGREDO-DA-ANA")) {
		t.Fatal("a importação de A deveria continuar intacta")
	}

	// e A vê B como membro
	var detalhe struct {
		Membros []struct{ Email string }
	}
	a.exigir("GET", "/api/casas/"+casa.ID, nil, 200).json(t, &detalhe)
	if len(detalhe.Membros) != 2 {
		t.Fatalf("casa deveria ter 2 membros: %+v", detalhe)
	}
}

func TestSessaoParcialNaoAcessaDados(t *testing.T) {
	amb := novoAmbiente(t)
	c := amb.novoCliente()
	c.exigir("POST", "/api/auth/cadastro", map[string]string{"nome": "Ana", "email": "ana@teste.com", "senha": "senha comprida de teste"}, http.StatusCreated)
	var ss struct {
		Autenticado       bool `json:"autenticado"`
		MFAOK             bool `json:"mfa_ok"`
		PrecisaConfigurar bool `json:"precisa_configurar_2fa"`
	}
	c.exigir("GET", "/api/auth/sessao", nil, 200).json(t, &ss)
	if !ss.Autenticado || ss.MFAOK || !ss.PrecisaConfigurar {
		t.Fatalf("sessão pós-cadastro: %+v", ss)
	}
	r := c.exigir("GET", "/api/entidades", nil, http.StatusForbidden)
	if !bytes.Contains(r.corpo, []byte("segundo_fator")) {
		t.Fatalf("esperava erro segundo_fator: %s", r.corpo)
	}
	c.exigir("POST", "/api/auth/sair", nil, http.StatusNoContent)
	c.exigir("GET", "/api/entidades", nil, http.StatusUnauthorized)
}

func TestProtecaoCSRF(t *testing.T) {
	amb := novoAmbiente(t)
	corpo := map[string]string{"email": "x@teste.com", "senha": "qualquer coisa longa"}

	semOrigem := amb.novoCliente()
	semOrigem.origem = ""
	semOrigem.exigir("POST", "/api/auth/entrar", corpo, http.StatusForbidden)

	outra := amb.novoCliente()
	outra.origem = "https://malicioso.example"
	outra.exigir("POST", "/api/auth/entrar", corpo, http.StatusForbidden)

	req := httptest.NewRequest("POST", "/api/auth/entrar", strings.NewReader("email=x&senha=y"))
	req.Header.Set("Origin", origemTeste)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	amb.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("formulário deveria ser recusado, veio %d", rec.Code)
	}
}

func TestCookieECabecalhos(t *testing.T) {
	amb := novoAmbiente(t)
	req := httptest.NewRequest("POST", "/api/auth/cadastro", strings.NewReader(`{"nome":"Ana","email":"ana@teste.com","senha":"senha comprida de teste"}`))
	req.Header.Set("Origin", origemTeste)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	amb.handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("cadastro: %d %s", rec.Code, rec.Body)
	}
	ck := rec.Header().Get("Set-Cookie")
	for _, atributo := range []string{"__Host-financas=", "HttpOnly", "Secure", "SameSite=Strict", "Path=/"} {
		if !strings.Contains(ck, atributo) {
			t.Errorf("cookie sem %s: %s", atributo, ck)
		}
	}
	csp := rec.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "script-src 'self'") || strings.Contains(csp, "unsafe") {
		t.Errorf("CSP: %s", csp)
	}
	if rec.Header().Get("Strict-Transport-Security") == "" || rec.Header().Get("X-Frame-Options") != "DENY" {
		t.Error("cabeçalhos de segurança ausentes")
	}

	// SPA: rota do front cai no index.html; asset inexistente é 404
	c := amb.novoCliente()
	if r := c.fazer("GET", "/casa", nil); r.status != 200 || !bytes.Contains(r.corpo, []byte("<title>Finanças")) {
		t.Errorf("SPA: %d %s", r.status, r.corpo)
	}
	if r := c.fazer("GET", "/assets/nao-existe.js", nil); r.status != 404 {
		t.Errorf("asset inexistente: %d", r.status)
	}
	if r := c.fazer("GET", "/api/nao-existe", nil); r.status != 404 || !bytes.Contains(r.corpo, []byte("nao_encontrado")) {
		t.Errorf("api inexistente: %d %s", r.status, r.corpo)
	}
}

func TestSaude(t *testing.T) {
	amb := novoAmbiente(t)
	c := amb.novoCliente()
	var h map[string]any
	c.exigir("GET", "/api/health", nil, http.StatusServiceUnavailable).json(t, &h)
	if h["db"] != "up" || h["worker"] != "down" || h["version"] != "v0.0.0-teste" {
		t.Fatalf("health sem worker: %v", h)
	}
	if err := worker.GravarSinal(context.Background(), amb.banco.App, "worker", nil); err != nil {
		t.Fatal(err)
	}
	c.exigir("GET", "/api/health", nil, http.StatusOK).json(t, &h)
	if h["status"] != "ok" || h["worker"] != "up" || h["pluggy"] != "ok" || h["backup"] != "pendente" {
		t.Fatalf("health: %v", h)
	}

	quebrado := novoAmbiente(t, func(c *config.Config) { c.ForcarFalhaSaude = true })
	_ = worker.GravarSinal(context.Background(), quebrado.banco.App, "worker", nil)
	quebrado.novoCliente().exigir("GET", "/api/health", nil, http.StatusServiceUnavailable)
}

func TestEntidades(t *testing.T) {
	amb := novoAmbiente(t)
	a := amb.novoCliente()
	a.cadastrarCom2FA("Ana", "ana@teste.com", "")

	a.exigir("POST", "/api/entidades", map[string]any{"tipo": "PF", "nome": "Ana", "documento": "111.111.111-11"}, http.StatusBadRequest)
	a.exigir("POST", "/api/entidades", map[string]any{"tipo": "PF", "nome": "Ana", "regime": "MEI"}, http.StatusBadRequest)
	var mei struct{ ID string }
	a.exigir("POST", "/api/entidades", map[string]any{
		"tipo": "PJ", "nome": "Ana Dev MEI", "documento": "11.222.333/0001-81", "regime": "MEI",
		"cnae": "6201-5/01", "data_abertura": "2024-03-01",
	}, http.StatusCreated).json(t, &mei)

	var e struct {
		Documento string `json:"documento"`
		Papel     string `json:"papel"`
		Regime    string `json:"regime"`
	}
	r := a.exigir("GET", "/api/entidades/"+mei.ID, nil, 200)
	r.json(t, &e)
	if e.Documento != "**.222.333/0001-**" || e.Papel != "dono" || e.Regime != "MEI" {
		t.Fatalf("entidade: %+v", e)
	}
	if bytes.Contains(r.corpo, []byte("11222333000181")) {
		t.Fatal("CNPJ completo não deveria sair na API")
	}
	var bruto string
	_ = amb.banco.Dono.QueryRow(context.Background(), "select documento_cifrado from entidades where id = $1", mei.ID).Scan(&bruto)
	if !strings.HasPrefix(bruto, "v1:") || strings.Contains(bruto, "11222333") {
		t.Fatalf("documento deveria estar cifrado no banco: %s", bruto)
	}

	// apagar exige reautenticação
	a.exigir("DELETE", "/api/entidades/"+mei.ID, nil, http.StatusForbidden)
	a.exigir("POST", "/api/auth/reautenticar", map[string]string{"senha": "senha comprida de teste"}, http.StatusNoContent)
	a.exigir("DELETE", "/api/entidades/"+mei.ID, nil, http.StatusNoContent)
	a.exigir("GET", "/api/entidades/"+mei.ID, nil, http.StatusNotFound)
	a.exigir("GET", "/api/entidades/nao-e-uuid", nil, http.StatusNotFound)
}

// Modo rede de casa: http://192.168.x.x, sem HTTPS. Cookie sem Secure (senão o
// navegador descarta), sem HSTS, sem passkey; login com senha + TOTP funciona.
func TestModoRedeDeCasa(t *testing.T) {
	const lan = "http://192.168.0.25:3100"
	banco := dbteste.Novo(t)
	cif, _ := cripto.Novo(bytes.Repeat([]byte{9}, 32))
	wa, err := auth.NovoWebAuthn(lan)
	if err != nil || wa != nil {
		t.Fatalf("com IP as passkeys deveriam ficar desligadas sem erro: %v %v", wa, err)
	}
	srv := &api.Servidor{
		Auth: &auth.Servico{Pool: banco.App, Cifrador: cif}, Pool: banco.App, Cifrador: cif,
		Config: config.Config{URLPublica: lan}, Versao: "teste",
	}
	amb := &ambiente{t: t, handler: srv.Handler(), banco: banco}
	c := amb.novoCliente()
	c.origem = lan

	r := c.exigir("POST", "/api/auth/cadastro", map[string]string{"nome": "Ana", "email": "ana@teste.com", "senha": "senha comprida de teste"}, http.StatusCreated)
	ck := r.cab.Get("Set-Cookie")
	if !strings.HasPrefix(ck, "financas=") || strings.Contains(ck, "Secure") || !strings.Contains(ck, "SameSite=Strict") {
		t.Fatalf("cookie no modo rede de casa: %s", ck)
	}
	if r.cab.Get("Strict-Transport-Security") != "" {
		t.Fatal("HSTS não pode ir em http")
	}
	c.exigir("POST", "/api/auth/passkey/registro/iniciar", nil, http.StatusBadRequest)

	var ini struct{ Segredo string }
	c.exigir("POST", "/api/auth/totp/iniciar", nil, http.StatusOK).json(t, &ini)
	codigo, _ := totp.GenerateCode(ini.Segredo, time.Now())
	c.exigir("POST", "/api/auth/totp/confirmar", map[string]string{"codigo": codigo}, http.StatusOK)
	c.exigir("GET", "/api/entidades", nil, http.StatusOK)

	// a checagem de origem continua valendo
	outro := amb.novoCliente()
	outro.origem = "http://192.168.0.99:3100"
	outro.exigir("POST", "/api/auth/entrar", map[string]string{"email": "ana@teste.com", "senha": "x"}, http.StatusForbidden)
}
