package ia

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// falsa responde /v1/messages com as respostas da fila e guarda os pedidos.
type falsa struct {
	mu        sync.Mutex
	respostas []string
	pedidos   []map[string]any
}

func (f *falsa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	corpo, _ := io.ReadAll(r.Body)
	var p map[string]any
	_ = json.Unmarshal(corpo, &p)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pedidos = append(f.pedidos, p)
	if r.URL.Path != "/v1/messages" || r.Header.Get("X-Api-Key") != "chave" || len(f.respostas) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"type":"error","error":{"type":"invalid_request_error","message":"pedido inesperado"}}`)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, f.respostas[0])
	f.respostas = f.respostas[1:]
}

func msg(stop string, uso string, blocos ...string) string {
	return `{"id":"msg_1","type":"message","role":"assistant","model":"x","stop_reason":"` + stop +
		`","content":[` + strings.Join(blocos, ",") + `],"usage":` + uso + `}`
}

func TestAnonimizar(t *testing.T) {
	casos := map[string]string{
		"PIX ENVIADO JOAO DA SILVA":                   "PIX ENVIADO [pessoa]",
		"Transferência recebida - Maria Souza":        "Transferência recebida [pessoa]",
		"PAG BOLETO 12345678 CNPJ 11.222.333/0001-81": "PAG BOLETO # CNPJ [cnpj]",
		"TED 529.982.247-25 AG 0001 CC 123456":        "TED [cpf] AG 0001 CC #",
		"email fulano@x.com.br":                       "email [email]",
		"SUPERMERCADO DIA":                            "SUPERMERCADO DIA",
		"UBER *TRIP":                                  "UBER *TRIP",
	}
	for entrada, quer := range casos {
		if s := Anonimizar(entrada); s != quer {
			t.Errorf("%q → %q, quer %q", entrada, s, quer)
		}
	}
}

func TestSemChave(t *testing.T) {
	var c *Cliente = Novo("  ", "")
	if c != nil {
		t.Fatal("sem chave, sem cliente")
	}
	if _, _, err := c.Categorizar(context.Background(), nil, nil); !errors.Is(err, ErrDesligada) {
		t.Fatal(err)
	}
	if _, err := c.Conversar(context.Background(), "", nil, nil); !errors.Is(err, ErrDesligada) {
		t.Fatal(err)
	}
}

func TestCategorizar(t *testing.T) {
	f := &falsa{respostas: []string{msg("tool_use",
		`{"input_tokens":1000,"output_tokens":200,"cache_creation_input_tokens":2000,"cache_read_input_tokens":0}`,
		`{"type":"tool_use","id":"tu_1","name":"classificar","input":{"itens":[
			{"t":"t1","categoria":"cat-mercado"},{"t":"t2","categoria":"nenhuma"},{"t":"t3","categoria":"inventada"}]}}`)}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := Novo("chave", srv.URL)
	itens := []ItemCategorizar{
		{ID: "uuid-1", Descricao: "SUPERMERCADO DIA 12345678", Valor: -8990},
		{ID: "uuid-2", Descricao: "PIX ENVIADO JOAO DA SILVA", Valor: -5000},
		{ID: "uuid-3", Descricao: "XPTO", Valor: -100},
	}
	cats := []CategoriaOpcao{{ID: "cat-mercado", Nome: "Alimentação › Mercado", Tipo: "gasto"}}
	res, uso, err := c.Categorizar(context.Background(), itens, cats)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res["uuid-1"] != "cat-mercado" {
		t.Fatalf("categorias: %+v", res)
	}
	// Haiku: 1.000 × $1 + 2.000 × $1,25 + 200 × $5 por milhão = US$ 0,0045 = 4.500 micro-dólares
	if uso.Chamadas != 1 || uso.Entrada != 3000 || uso.Saida != 200 || uso.CustoMicro != 4500 {
		t.Fatalf("uso: %+v", uso)
	}
	p := f.pedidos[0]
	texto, _ := json.Marshal(p["messages"])
	if strings.Contains(string(texto), "JOAO") || strings.Contains(string(texto), "12345678") || strings.Contains(string(texto), "uuid-") {
		t.Fatalf("dado pessoal ou id interno saiu do Pi: %s", texto)
	}
	if p["model"] != ModeloCategorizar {
		t.Fatalf("modelo: %v", p["model"])
	}
	// nada a categorizar: nenhuma chamada
	if res, _, err := c.Categorizar(context.Background(), nil, cats); err != nil || len(res) != 0 || len(f.pedidos) != 1 {
		t.Fatal("lista vazia não chama a API")
	}
}

func TestCategorizarErros(t *testing.T) {
	f := &falsa{respostas: []string{msg("tool_use", `{"input_tokens":1,"output_tokens":1}`,
		`{"type":"tool_use","id":"tu_1","name":"classificar","input":{"itens":"x"}}`)}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := Novo("chave", srv.URL)
	itens := []ItemCategorizar{{ID: "a", Descricao: "x", Valor: -1}}
	cats := []CategoriaOpcao{{ID: "c", Nome: "C", Tipo: "gasto"}}
	if _, _, err := c.Categorizar(context.Background(), itens, cats); err == nil {
		t.Fatal("entrada inválida da ferramenta")
	}
	// fila vazia: a API falsa responde 400
	if _, _, err := c.Categorizar(context.Background(), itens, cats); err == nil {
		t.Fatal("erro da API")
	}
}

func TestConversar(t *testing.T) {
	f := &falsa{respostas: []string{
		msg("tool_use", `{"input_tokens":100,"output_tokens":10}`,
			`{"type":"text","text":"Vou ver."}`,
			`{"type":"tool_use","id":"tu_1","name":"gastos_por_categoria","input":{"mes":"2026-09"}}`,
			`{"type":"tool_use","id":"tu_2","name":"quebra","input":{}}`,
			`{"type":"tool_use","id":"tu_3","name":"nao_existe","input":{}}`),
		msg("end_turn", `{"input_tokens":200,"output_tokens":20}`,
			`{"type":"text","text":"Você gastou R$ 89,90 em Mercado em setembro."}`),
	}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := Novo("chave", srv.URL)
	var recebido string
	ferr := []Ferramenta{
		{Nome: "gastos_por_categoria", Descricao: "gastos", Parametros: map[string]any{"mes": map[string]any{"type": "string"}},
			Rodar: func(_ context.Context, e json.RawMessage) (any, error) {
				recebido = string(e)
				return map[string]any{"Mercado": 8990}, nil
			}},
		{Nome: "quebra", Descricao: "sempre falha", Parametros: map[string]any{},
			Rodar: func(context.Context, json.RawMessage) (any, error) { return nil, errors.New("falhou") }},
	}
	r, err := c.Conversar(context.Background(), "2026-09-26",
		[]Mensagem{{Papel: "usuario", Texto: "Quanto gastei de mercado?"}}, ferr)
	if err != nil {
		t.Fatal(err)
	}
	if r.Texto != "Você gastou R$ 89,90 em Mercado em setembro." || len(r.Ferramentas) != 3 || r.Uso.Chamadas != 2 ||
		!strings.Contains(recebido, "2026-09") {
		t.Fatalf("resposta: %+v (%s)", r, recebido)
	}
	// o segundo pedido leva os três resultados numa mensagem só, com os erros marcados
	seg, _ := json.Marshal(f.pedidos[1]["messages"])
	s := string(seg)
	if !strings.Contains(s, `"is_error":true`) || !strings.Contains(s, "8990") || !strings.Contains(s, "hoje é 2026-09-26") ||
		!strings.Contains(s, "ferramenta desconhecida") {
		t.Fatalf("segundo pedido: %s", s)
	}
	if f.pedidos[0]["model"] != ModeloChat {
		t.Fatalf("modelo: %v", f.pedidos[0]["model"])
	}
}

func TestConversarLimiteERecusa(t *testing.T) {
	var fila []string
	for range MaxPassos {
		fila = append(fila, msg("tool_use", `{"input_tokens":1,"output_tokens":1}`,
			`{"type":"tool_use","id":"tu","name":"eco","input":{}}`))
	}
	fila = append(fila, msg("refusal", `{"input_tokens":1,"output_tokens":1}`))
	f := &falsa{respostas: fila}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := Novo("chave", srv.URL)
	eco := []Ferramenta{{Nome: "eco", Parametros: map[string]any{},
		Rodar: func(context.Context, json.RawMessage) (any, error) { return 1, nil }}}
	hist := []Mensagem{{Papel: "usuario", Texto: "oi"}, {Papel: "assistente", Texto: "olá"}, {Papel: "usuario", Texto: "e aí"}}
	r, err := c.Conversar(context.Background(), "2026-09-26", hist, eco)
	if err != nil || !r.Interrompida || r.Uso.Chamadas != MaxPassos {
		t.Fatalf("limite: %+v %v", r, err)
	}
	r, err = c.Conversar(context.Background(), "2026-09-26", hist, eco)
	if err != nil || r.Texto != "Não posso responder isso." {
		t.Fatalf("recusa: %+v %v", r, err)
	}
	// erro da API (fila vazia)
	if _, err := c.Conversar(context.Background(), "2026-09-26", hist, eco); err == nil {
		t.Fatal("erro da API")
	}
	var total Uso
	total.Somar(Uso{Chamadas: 1, Entrada: 2, Saida: 3, CustoMicro: 4})
	if total.CustoMicro != 4 || total.Entrada != 2 {
		t.Fatal("somar uso")
	}
}

func TestEscreverRelatorio(t *testing.T) {
	f := &falsa{respostas: []string{msg("end_turn", `{"input_tokens":500,"output_tokens":100}`,
		`{"type":"text","text":" Você gastou menos. "}`)}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := Novo("chave", srv.URL)
	texto, uso, err := c.EscreverRelatorio(context.Background(), map[string]any{"gastos": "R$ 10,00"})
	if err != nil || texto != "Você gastou menos." || uso.Chamadas != 1 || f.pedidos[0]["model"] != ModeloChat {
		t.Fatalf("%q %+v %v", texto, uso, err)
	}
	if _, _, err := c.EscreverRelatorio(context.Background(), map[string]any{}); err == nil {
		t.Fatal("erro da API")
	}
	if _, _, err := c.EscreverRelatorio(context.Background(), func() {}); err == nil {
		t.Fatal("dados que não viram JSON")
	}
	var nulo *Cliente
	if _, _, err := nulo.EscreverRelatorio(context.Background(), nil); !errors.Is(err, ErrDesligada) {
		t.Fatal(err)
	}
}

func TestLerDocumento(t *testing.T) {
	f := &falsa{respostas: []string{
		msg("tool_use", `{"input_tokens":3000,"output_tokens":200}`,
			`{"type":"tool_use","id":"tu","name":"extrair","input":{"data":"2026-09-20","descricao":"Padaria","valor":"12.50","sentido":"saida"}}`),
		msg("end_turn", `{"input_tokens":1,"output_tokens":1}`, `{"type":"text","text":"não li"}`),
	}}
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := Novo("chave", srv.URL)
	j, uso, err := c.LerDocumento(context.Background(), DocComprovante, "image/png", []byte("\x89PNG..."))
	if err != nil || !strings.Contains(string(j), `"valor":"12.50"`) || uso.Chamadas != 1 {
		t.Fatalf("%s %+v %v", j, uso, err)
	}
	pedido, _ := json.Marshal(f.pedidos[0])
	if !strings.Contains(string(pedido), `"type":"image"`) || f.pedidos[0]["model"] != ModeloCategorizar {
		t.Fatalf("pedido: %s", pedido)
	}
	if _, _, err := c.LerDocumento(context.Background(), DocFatura, "application/pdf", []byte("%PDF-1.4")); err == nil {
		t.Fatal("sem a ferramenta, erro")
	}
	pedido, _ = json.Marshal(f.pedidos[1])
	if !strings.Contains(string(pedido), `"type":"document"`) {
		t.Fatalf("pdf: %s", pedido)
	}
	if _, _, err := c.LerDocumento(context.Background(), DocNota, "application/pdf", []byte("%PDF")); err == nil {
		t.Fatal("erro da API")
	}
	for _, caso := range [][2]string{{"x", "application/pdf"}, {DocHolerite, "text/html"}} {
		if _, _, err := c.LerDocumento(context.Background(), caso[0], caso[1], []byte("x")); !errors.Is(err, ErrDocumento) {
			t.Fatalf("%v: %v", caso, err)
		}
	}
	if _, _, err := c.LerDocumento(context.Background(), DocHolerite, "application/pdf", nil); !errors.Is(err, ErrDocumento) {
		t.Fatal("vazio")
	}
	var nulo *Cliente
	if _, _, err := nulo.LerDocumento(context.Background(), DocFatura, "application/pdf", []byte("x")); !errors.Is(err, ErrDesligada) {
		t.Fatal(err)
	}
}
