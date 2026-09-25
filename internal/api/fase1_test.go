package api_test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yarion1/pi-finance/internal/api"
)

// ofx monta um extrato OFX 1.x com as transações dadas (data, valor, id, descrição).
func ofx(linhas ...[4]string) string {
	var b strings.Builder
	b.WriteString("OFXHEADER:100\nDATA:OFXSGML\n\n<OFX><BANKMSGSRSV1><STMTTRNRS><STMTRS><CURDEF>BRL<BANKTRANLIST>\n")
	for _, l := range linhas {
		fmt.Fprintf(&b, "<STMTTRN>\n<TRNTYPE>OTHER\n<DTPOSTED>%s\n<TRNAMT>%s\n<FITID>%s\n<MEMO>%s\n</STMTTRN>\n", l[0], l[1], l[2], l[3])
	}
	b.WriteString("</BANKTRANLIST></STMTRS></STMTTRNRS></BANKMSGSRSV1></OFX>\n")
	return base64.StdEncoding.EncodeToString([]byte(b.String()))
}

func ofxBase64(desc, valor, id string) string { return ofx([4]string{"20260905", valor, id, desc}) }

type resultadoImp struct {
	ImportacaoID   string `json:"importacao_id"`
	Linhas         int    `json:"linhas"`
	Novas          int    `json:"novas"`
	Duplicadas     int    `json:"duplicadas"`
	Transferencias int    `json:"transferencias"`
	Categorizadas  int    `json:"categorizadas"`
	Previa         []struct {
		Duplicada bool `json:"duplicada"`
	} `json:"previa"`
}

type listaTransacoes struct {
	Itens []struct {
		ID          string  `json:"id"`
		Descricao   string  `json:"descricao"`
		Valor       int64   `json:"valor_centavos"`
		Tipo        string  `json:"tipo"`
		CategoriaID *string `json:"categoria_id"`
		Categoria   *string `json:"categoria"`
		Par         *string `json:"transferencia_par_id"`
	} `json:"itens"`
	Total int `json:"total"`
}

// prepara: Ana com PF e duas contas (Nubank e Inter).
func prepara(t *testing.T) (*ambiente, *cliente, string, string, string) {
	t.Helper()
	amb := novoAmbiente(t)
	a := amb.novoCliente()
	a.cadastrarCom2FA("Ana", "ana@teste.com", "")
	var pf, nu, inter struct{ ID string }
	a.exigir("POST", "/api/entidades", map[string]any{"tipo": "PF", "nome": "Ana PF"}, http.StatusCreated).json(t, &pf)
	a.exigir("POST", "/api/contas", map[string]any{"entidade_id": pf.ID, "nome": "Nubank", "tipo": "corrente", "saldo_inicial_centavos": 100000}, http.StatusCreated).json(t, &nu)
	a.exigir("POST", "/api/contas", map[string]any{"entidade_id": pf.ID, "nome": "Inter", "tipo": "corrente"}, http.StatusCreated).json(t, &inter)
	return amb, a, pf.ID, nu.ID, inter.ID
}

// Critério de aceite da fase 1: importar o mesmo OFX duas vezes não duplica nada.
func TestImportarMesmoOFXDuasVezes(t *testing.T) {
	_, a, _, nu, _ := prepara(t)
	arquivo := ofx(
		[4]string{"20260905", "-45.90", "f1", "Padaria"},
		[4]string{"20260905", "-45.90", "f2", "Padaria"}, // duas compras iguais no dia: são duas
		[4]string{"20260910", "5000.00", "f3", "Salário EMPRESA"},
	)
	corpo := map[string]any{"conta_id": nu, "arquivo": "set.ofx", "conteudo": arquivo}

	var previa resultadoImp
	a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": nu, "arquivo": "set.ofx", "conteudo": arquivo, "simular": true}, http.StatusOK).json(t, &previa)
	if previa.Novas != 3 || len(previa.Previa) != 3 || previa.ImportacaoID != "" {
		t.Fatalf("prévia: %+v", previa)
	}

	var r1, r2 resultadoImp
	a.exigir("POST", "/api/importacoes", corpo, http.StatusCreated).json(t, &r1)
	a.exigir("POST", "/api/importacoes", corpo, http.StatusCreated).json(t, &r2)
	if r1.Novas != 3 || r1.Duplicadas != 0 || r2.Novas != 0 || r2.Duplicadas != 3 {
		t.Fatalf("1ª: %+v / 2ª: %+v", r1, r2)
	}
	var l listaTransacoes
	a.exigir("GET", "/api/transacoes?conta_id="+nu, nil, http.StatusOK).json(t, &l)
	if l.Total != 3 {
		t.Fatalf("esperava 3 transações, há %d", l.Total)
	}

	// o mesmo extrato como CSV do Nubank (sem id externo) também não duplica
	csv := base64.StdEncoding.EncodeToString([]byte("Data,Valor,Identificador,Descrição\n05/09/2026,-45.90,,Padaria\n05/09/2026,-45.90,,Padaria\n10/09/2026,5000.00,,Salário EMPRESA\n"))
	var r3 resultadoImp
	a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": nu, "arquivo": "set.csv", "conteudo": csv}, http.StatusCreated).json(t, &r3)
	if r3.Novas != 0 || r3.Duplicadas != 3 {
		t.Fatalf("CSV sobreposto: %+v", r3)
	}

	// saldo = inicial + transações
	var contas []struct {
		ID    string `json:"id"`
		Saldo int64  `json:"saldo_centavos"`
	}
	a.exigir("GET", "/api/contas", nil, http.StatusOK).json(t, &contas)
	for _, c := range contas {
		if c.ID == nu && c.Saldo != 100000-4590*2+500000 {
			t.Fatalf("saldo do Nubank: %d", c.Saldo)
		}
	}

	// desfazer apaga tudo da importação
	var desf struct{ Apagadas int }
	a.exigir("DELETE", "/api/importacoes/"+r1.ImportacaoID, nil, http.StatusOK).json(t, &desf)
	a.exigir("GET", "/api/transacoes?conta_id="+nu, nil, http.StatusOK).json(t, &l)
	if desf.Apagadas != 3 || l.Total != 0 {
		t.Fatalf("desfazer: %+v, restam %d", desf, l.Total)
	}
	// e depois de desfeita, o arquivo entra de novo
	var r4 resultadoImp
	a.exigir("POST", "/api/importacoes", corpo, http.StatusCreated).json(t, &r4)
	if r4.Novas != 3 {
		t.Fatalf("reimportar depois de desfazer: %+v", r4)
	}
}

// Critério de aceite da fase 1: transferência entre contas não aparece como gasto.
func TestTransferenciaNaoEGasto(t *testing.T) {
	_, a, pf, nu, inter := prepara(t)
	mes := time.Now().Format("2006-01")
	hoje := time.Now().Format("20060102")
	ontem := time.Now().AddDate(0, 0, -1)
	if ontem.Format("2006-01") != mes {
		ontem = time.Now()
	}

	var r1, r2 resultadoImp
	a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": nu, "arquivo": "nu.ofx", "conteudo": ofx(
		[4]string{ontem.Format("20060102"), "-1200.00", "n1", "Pix enviado Ana Inter"},
		[4]string{hoje, "-80.00", "n2", "Mercado"},
	)}, http.StatusCreated).json(t, &r1)
	a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": inter, "arquivo": "inter.ofx", "conteudo": ofx(
		[4]string{hoje, "1200.00", "i1", "Pix recebido Ana Nubank"},
	)}, http.StatusCreated).json(t, &r2)
	if r2.Transferencias != 1 {
		t.Fatalf("esperava 1 transferência pareada: %+v", r2)
	}

	var l listaTransacoes
	a.exigir("GET", "/api/transacoes?entidade_id="+pf, nil, http.StatusOK).json(t, &l)
	pares := 0
	for _, tr := range l.Itens {
		if strings.HasPrefix(tr.Descricao, "Pix") {
			if tr.Tipo != "transferencia" || tr.Par == nil || tr.Categoria == nil || *tr.Categoria != "Transferência entre contas" {
				t.Errorf("%s: %+v", tr.Descricao, tr)
			}
			pares++
		}
	}
	if pares != 2 {
		t.Fatalf("esperava as duas pontas: %d", pares)
	}

	var res struct {
		Gastos   int64 `json:"gastos_centavos"`
		Receitas int64 `json:"receitas_centavos"`
		Saldo    int64 `json:"saldo_total_centavos"`
	}
	a.exigir("GET", "/api/resumo?mes="+mes, nil, http.StatusOK).json(t, &res)
	if res.Gastos != 8000 || res.Receitas != 0 {
		t.Fatalf("a transferência entrou no resumo: gastos %d, receitas %d", res.Gastos, res.Receitas)
	}
	if res.Saldo != 100000-8000 {
		t.Fatalf("saldo total %d", res.Saldo)
	}

	// desfazer a importação do Inter devolve o Pix do Nubank a gasto comum
	a.exigir("DELETE", "/api/importacoes/"+r2.ImportacaoID, nil, http.StatusOK)
	a.exigir("GET", "/api/resumo?mes="+mes, nil, http.StatusOK).json(t, &res)
	if res.Gastos != 128000 {
		t.Fatalf("sem a outra ponta o Pix volta a ser gasto: %d", res.Gastos)
	}
}

func TestCategorizacao(t *testing.T) {
	_, a, pf, nu, _ := prepara(t)
	var cats []struct {
		ID, Nome, Tipo string
		PaiID          *string `json:"pai_id"`
	}
	a.exigir("GET", "/api/categorias", nil, http.StatusOK).json(t, &cats)
	id := func(nome string) string {
		for _, c := range cats {
			if c.Nome == nome {
				return c.ID
			}
		}
		t.Fatalf("categoria %s não existe", nome)
		return ""
	}
	app, mercado := id("App"), id("Mercado")

	// regra: "uber" → Transporte › App
	a.exigir("POST", "/api/regras", map[string]any{"entidade_id": pf, "texto": "uber", "categoria_id": app}, http.StatusCreated)

	var r resultadoImp
	a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": nu, "arquivo": "a.ofx", "conteudo": ofx(
		[4]string{"20260901", "-23.45", "u1", "UBER *TRIP 8812"},
		[4]string{"20260902", "-312.40", "m1", "SUPERMERCADO BOM PRECO 0231"},
	)}, http.StatusCreated).json(t, &r)
	if r.Categorizadas != 1 {
		t.Fatalf("só o Uber tem regra: %+v", r)
	}

	// o usuário categoriza o mercado (edição em massa); o histórico aprende
	var l listaTransacoes
	a.exigir("GET", "/api/transacoes?sem_categoria=1&conta_id="+nu, nil, http.StatusOK).json(t, &l)
	if l.Total != 1 {
		t.Fatalf("sem categoria: %d", l.Total)
	}
	a.exigir("POST", "/api/transacoes/lote", map[string]any{"ids": []string{l.Itens[0].ID}, "categoria_id": mercado}, http.StatusNoContent)

	a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": nu, "arquivo": "b.ofx", "conteudo": ofx(
		[4]string{"20260915", "-150.00", "m2", "SUPERMERCADO BOM PRECO 0877"},
		[4]string{"20260916", "-18.00", "u2", "Uber *trip 1290"},
	)}, http.StatusCreated).json(t, &r)
	if r.Categorizadas != 2 {
		t.Fatalf("histórico e regra deveriam categorizar as duas: %+v", r)
	}

	// categoria de gasto numa entrada vira estorno; de receita numa saída é recusada
	var man struct{ ID string }
	a.exigir("POST", "/api/transacoes", map[string]any{"conta_id": nu, "data": "2026-09-17", "descricao": "Estorno loja", "valor_centavos": 5000}, http.StatusCreated).json(t, &man)
	a.exigir("PATCH", "/api/transacoes/"+man.ID, map[string]any{"alterar_categoria": true, "categoria_id": id("Roupas")}, http.StatusNoContent)
	a.exigir("GET", "/api/transacoes?busca=Estorno", nil, http.StatusOK).json(t, &l)
	if l.Itens[0].Tipo != "estorno" {
		t.Fatalf("tipo: %s", l.Itens[0].Tipo)
	}
	a.exigir("PATCH", "/api/transacoes/"+l.Itens[0].ID, map[string]any{"alterar_categoria": true, "categoria_id": id("Salário")}, http.StatusNoContent)
	var saida struct{ ID string }
	a.exigir("POST", "/api/transacoes", map[string]any{"conta_id": nu, "data": "2026-09-17", "descricao": "x", "valor_centavos": -100}, http.StatusCreated).json(t, &saida)
	a.exigir("PATCH", "/api/transacoes/"+saida.ID, map[string]any{"alterar_categoria": true, "categoria_id": id("Salário")}, http.StatusBadRequest)

	// aplicar regras em lote nas antigas
	a.exigir("POST", "/api/regras", map[string]any{"entidade_id": pf, "texto": "x", "categoria_id": id("Outros gastos")}, http.StatusCreated)
	var ap struct{ Categorizadas int }
	a.exigir("POST", "/api/regras/aplicar", map[string]any{"entidade_id": pf}, http.StatusOK).json(t, &ap)
	if ap.Categorizadas != 1 {
		t.Fatalf("aplicar regras: %+v", ap)
	}
}

func TestCSVGenericoPedeMapeamento(t *testing.T) {
	_, a, _, nu, _ := prepara(t)
	csv := base64.StdEncoding.EncodeToString([]byte("Quando;O que;Quanto\n10/09/2026;Mercado;-312,40\n"))
	r := a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": nu, "arquivo": "x.csv", "conteudo": csv}, http.StatusUnprocessableEntity)
	if !strings.Contains(string(r.corpo), `"cabecalhos":["Quando","O que","Quanto"]`) {
		t.Fatalf("deveria devolver os cabeçalhos: %s", r.corpo)
	}
	mapa := map[string]any{"coluna_data": "Quando", "colunas_descricao": []string{"O que"}, "coluna_valor": "Quanto", "formato_data": "dd/mm/aaaa", "decimal": ","}
	var res resultadoImp
	a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": nu, "arquivo": "x.csv", "conteudo": csv, "mapeamento": mapa}, http.StatusCreated).json(t, &res)
	if res.Novas != 1 {
		t.Fatalf("%+v", res)
	}
	a.exigir("POST", "/api/mapeamentos", map[string]any{"nome": "Banco X", "config": mapa}, http.StatusCreated)
	var ms []struct{ Nome string }
	a.exigir("GET", "/api/mapeamentos", nil, http.StatusOK).json(t, &ms)
	if len(ms) != 1 || ms[0].Nome != "Banco X" {
		t.Fatalf("mapeamentos: %+v", ms)
	}
}

// A casa vê o saldo de uma conta "só saldo", mas não as transações.
func TestCasaVeSoSaldo(t *testing.T) {
	amb, a, pf, nu, _ := prepara(t)
	var casa struct{ ID string }
	a.exigir("POST", "/api/casas", map[string]string{"nome": "Casa"}, http.StatusCreated).json(t, &casa)
	a.exigir("PATCH", "/api/contas/"+nu, map[string]any{"nome": "Nubank", "tipo": "corrente", "saldo_inicial_centavos": 100000,
		"visibilidade": "saldo", "casa_id": casa.ID}, http.StatusNoContent)
	a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": nu, "arquivo": "a.ofx", "conteudo": ofxBase64("COMPRA-PRIVADA", "-10.00", "p1")}, http.StatusCreated)
	var conv struct{ Link string }
	a.exigir("POST", "/api/casas/"+casa.ID+"/convites", map[string]string{"papel": "membro"}, http.StatusCreated).json(t, &conv)

	b := amb.novoCliente()
	b.cadastrarCom2FA("Bruno", "bruno@teste.com", conv.Link[strings.LastIndex(conv.Link, "/")+1:])
	var contas []struct {
		Nome         string  `json:"nome"`
		Saldo        *int64  `json:"saldo_centavos"`
		Dono         *string `json:"dono"`
		EntidadeNome *string `json:"entidade_nome"`
		PodeEditar   bool    `json:"pode_editar"`
	}
	b.exigir("GET", "/api/contas", nil, http.StatusOK).json(t, &contas)
	if len(contas) != 1 || contas[0].Saldo == nil || *contas[0].Saldo != 99000 || contas[0].Dono == nil || *contas[0].Dono != "Ana" ||
		contas[0].EntidadeNome != nil || contas[0].PodeEditar {
		t.Fatalf("B deveria ver só o saldo e de quem é: %+v", contas)
	}
	var l listaTransacoes
	b.exigir("GET", "/api/transacoes", nil, http.StatusOK).json(t, &l)
	if l.Total != 0 {
		t.Fatal("B não pode ver transações de conta só saldo")
	}
	_ = pf
	_ = api.RotasLeitura
}
