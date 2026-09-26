package api_test

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestLerELancarDocumentos(t *testing.T) {
	amb, a, pf, nu, _ := prepara(t)
	pdf := base64.StdEncoding.EncodeToString([]byte("%PDF-1.4 fatura de teste"))
	png := base64.StdEncoding.EncodeToString([]byte("\x89PNG\r\n\x1a\nfoto"))
	var extraido string
	amb.claude.responde = func(p map[string]any) string {
		return respostaClaude("tool_use", `{"type":"tool_use","id":"tu","name":"extrair","input":`+extraido+`}`)
	}
	ler := func(tipo, conteudo string, status int) previaDoc {
		var p previaDoc
		a.exigir("POST", "/api/documentos/ler", map[string]any{"tipo": tipo, "conteudo": conteudo, "envio_confirmado": true},
			status).json(t, &p)
		return p
	}

	// sem confirmar o envio, sem IA ligada, arquivo que não é PDF nem foto, tipo errado
	a.exigir("POST", "/api/documentos/ler", map[string]any{"tipo": "fatura", "conteudo": pdf}, http.StatusBadRequest)
	ler("fatura", pdf, http.StatusBadRequest) // IA desligada
	ligarIA(t, a, 5_000_000)
	ler("fatura", base64.StdEncoding.EncodeToString([]byte("<html>")), http.StatusBadRequest)
	ler("recibo", pdf, http.StatusBadRequest)
	if len(amb.claude.pedidos) != 0 {
		t.Fatal("nada deveria ter ido para a IA")
	}

	// fatura: prévia, simulação e lançamento pela importação de sempre (sem duplicar)
	extraido = `{"vencimento":"2026-10-10","lancamentos":[{"data":"2026-09-02","descricao":"LOJA X","valor":"300.00","parcela":"2/6"},
		{"data":"2026-09-03","descricao":"PADARIA","valor":"12.00"}]}`
	p := ler("fatura", pdf, http.StatusOK)
	if len(p.Linhas) != 2 || p.Linhas[0].Descricao != "LOJA X - Parcela 2/6" || p.Linhas[0].Valor != -30000 {
		t.Fatalf("prévia: %+v", p)
	}
	var imp resultadoImp
	a.exigir("POST", "/api/documentos/lancar", map[string]any{"tipo": "fatura", "conta_id": nu, "extraido": p.Extraido, "simular": true},
		http.StatusOK).json(t, &imp)
	if len(imp.Previa) != 2 {
		t.Fatalf("simulação: %+v", imp)
	}
	a.exigir("POST", "/api/documentos/lancar", map[string]any{"tipo": "fatura", "conta_id": nu, "extraido": p.Extraido},
		http.StatusOK).json(t, &imp)
	if imp.Novas != 2 {
		t.Fatalf("lançamento: %+v", imp)
	}
	a.exigir("POST", "/api/documentos/lancar", map[string]any{"tipo": "fatura", "conta_id": nu, "extraido": p.Extraido},
		http.StatusOK).json(t, &imp)
	if imp.Novas != 0 || imp.Duplicadas != 2 {
		t.Fatalf("de novo: %+v", imp)
	}
	var l listaTransacoes
	a.exigir("GET", "/api/transacoes?busca=PADARIA", nil, http.StatusOK).json(t, &l)
	if l.Total != 1 {
		t.Fatalf("transação: %+v", l)
	}

	// comprovante por foto
	extraido = `{"data":"2026-09-20","descricao":"Pix para a diarista","valor":"150.00","sentido":"saida"}`
	p = ler("comprovante", png, http.StatusOK)
	if len(p.Linhas) != 1 || p.Linhas[0].Valor != -15000 {
		t.Fatalf("comprovante: %+v", p)
	}
	pedido := amb.claude.pedidos[len(amb.claude.pedidos)-1]
	if pedido["model"] != "claude-haiku-4-5" || !strings.Contains(strings.ToLower(jsonTexto(pedido)), `"type":"image"`) {
		t.Fatalf("pedido: %s", jsonTexto(pedido))
	}

	// nota de corretagem: operações com as taxas rateadas
	extraido = `{"data_pregao":"2026-09-15","corretora":"XP","taxas_total":"3.00","operacoes":[
		{"codigo":"PETR4","tipo":"C","quantidade":"10","preco":"30.00"}]}`
	p = ler("nota_corretagem", pdf, http.StatusOK)
	if len(p.Operacoes) != 1 {
		t.Fatalf("nota: %+v", p)
	}
	a.exigir("POST", "/api/documentos/lancar", map[string]any{"tipo": "nota_corretagem", "entidade_id": pf, "extraido": p.Extraido},
		http.StatusOK)
	var inv struct {
		Ativos []struct {
			Codigo string `json:"codigo"`
			Custo  int64  `json:"custo_centavos"`
		} `json:"ativos"`
	}
	a.exigir("GET", "/api/investimentos", nil, http.StatusOK).json(t, &inv)
	if len(inv.Ativos) != 1 || inv.Ativos[0].Codigo != "PETR4" || inv.Ativos[0].Custo != 30300 {
		t.Fatalf("carteira: %+v", inv)
	}

	// holerite
	extraido = `{"competencia":"2026-08","empregador":"ACME","bruto":"5000.00","inss":"501.51","irrf":"0","liquido":"4498.49"}`
	p = ler("holerite", pdf, http.StatusOK)
	if p.Holerite == nil || p.Holerite.Bruto != 500000 {
		t.Fatalf("holerite: %+v", p)
	}
	a.exigir("POST", "/api/documentos/lancar", map[string]any{"tipo": "holerite", "entidade_id": pf, "extraido": p.Extraido}, http.StatusOK)
	a.exigir("POST", "/api/documentos/lancar", map[string]any{"tipo": "holerite", "entidade_id": pf, "extraido": p.Extraido}, http.StatusOK)
	var hol struct {
		Itens []struct{ ID string } `json:"itens"`
		Bruto int64                 `json:"bruto_centavos"`
		INSS  int64                 `json:"inss_centavos"`
	}
	a.exigir("GET", "/api/holerites?ano=2026", nil, http.StatusOK).json(t, &hol)
	if len(hol.Itens) != 1 || hol.Bruto != 500000 || hol.INSS != 50151 {
		t.Fatalf("holerites: %+v", hol)
	}
	a.exigir("GET", "/api/holerites?ano=x", nil, http.StatusBadRequest)
	a.exigir("POST", "/api/documentos/lancar", map[string]any{"tipo": "holerite", "entidade_id": pf, "extraido": map[string]string{}}, http.StatusBadRequest)

	// a IA falhou: 502, e nada lançado
	amb.claude.responde = func(map[string]any) string { return respostaClaude("end_turn", `{"type":"text","text":"?"}`) }
	ler("fatura", pdf, http.StatusBadGateway)

	// outra pessoa não lança na conta nem apaga o holerite de Ana
	var casa struct{ ID string }
	a.exigir("POST", "/api/casas", map[string]string{"nome": "Casa"}, http.StatusCreated).json(t, &casa)
	var convite struct{ Link string }
	a.exigir("POST", "/api/casas/"+casa.ID+"/convites", map[string]string{"papel": "membro"}, http.StatusCreated).json(t, &convite)
	b := amb.novoCliente()
	b.cadastrarCom2FA("Bruno", "bruno@teste.com", convite.Link[strings.LastIndex(convite.Link, "/")+1:])
	b.exigir("POST", "/api/documentos/lancar", map[string]any{"tipo": "comprovante", "conta_id": nu,
		"extraido": map[string]string{"data": "2026-09-20", "descricao": "x", "valor": "1", "sentido": "saida"}}, http.StatusNotFound)
	b.exigir("DELETE", "/api/holerites/"+hol.Itens[0].ID, nil, http.StatusNotFound)
	a.exigir("DELETE", "/api/holerites/"+hol.Itens[0].ID, nil, http.StatusNoContent)
}

type previaDoc struct {
	Extraido map[string]any `json:"extraido"`
	Linhas   []struct {
		Descricao string `json:"descricao"`
		Valor     int64  `json:"valor_centavos"`
	} `json:"linhas"`
	Operacoes []map[string]any `json:"operacoes"`
	Holerite  *struct {
		Bruto int64 `json:"bruto_centavos"`
	} `json:"holerite"`
}

func jsonTexto(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
