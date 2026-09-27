package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/yarion1/pi-finance/internal/financeiro"
	"github.com/yarion1/pi-finance/internal/worker"
)

func TestExportarApagarEAtividade(t *testing.T) {
	amb, a, _, nu, _ := prepara(t)
	ctx := context.Background()
	hoje := financeiro.Hoje().Format("2006-01-02")
	var pj struct{ ID string }
	a.exigir("POST", "/api/entidades", map[string]any{"tipo": "PF", "nome": "Ana com CPF", "documento": "529.982.247-25"},
		http.StatusCreated).json(t, &pj)
	a.exigir("POST", "/api/transacoes", map[string]any{"conta_id": nu, "data": hoje, "descricao": "PADARIA DA ANA",
		"valor_centavos": -1234}, http.StatusCreated)
	var casa struct{ ID string }
	a.exigir("POST", "/api/casas", map[string]string{"nome": "Casa"}, http.StatusCreated).json(t, &casa)
	var conv struct{ Link string }
	a.exigir("POST", "/api/casas/"+casa.ID+"/convites", map[string]string{"papel": "membro"}, http.StatusCreated).json(t, &conv)
	b := amb.novoCliente()
	b.cadastrarCom2FA("Bia", "bia@teste.com", conv.Link[strings.LastIndex(conv.Link, "/")+1:])
	var pfB, contaB struct{ ID string }
	b.exigir("POST", "/api/entidades", map[string]any{"tipo": "PF", "nome": "Bia PF"}, http.StatusCreated).json(t, &pfB)
	b.exigir("POST", "/api/contas", map[string]any{"entidade_id": pfB.ID, "nome": "Conta da Bia", "tipo": "corrente"},
		http.StatusCreated).json(t, &contaB)
	b.exigir("POST", "/api/transacoes", map[string]any{"conta_id": contaB.ID, "data": hoje, "descricao": "SEGREDO-DA-BIA",
		"valor_centavos": -999}, http.StatusCreated)

	// exportar pede a senha de novo
	a.exigir("POST", "/api/conta/exportar", map[string]string{"formato": "json"}, http.StatusForbidden)
	a.exigir("POST", "/api/auth/reautenticar", map[string]string{"senha": "senha comprida de teste"}, http.StatusNoContent)
	a.exigir("POST", "/api/conta/exportar", map[string]string{"formato": "pdf"}, http.StatusBadRequest)
	r := a.exigir("POST", "/api/conta/exportar", map[string]string{"formato": "json"}, http.StatusOK)
	if !strings.Contains(r.cab.Get("Content-Disposition"), `.json"`) {
		t.Fatalf("cabeçalho: %v", r.cab)
	}
	var exp struct {
		Usuario   map[string]any
		Entidades []map[string]any
		Tabelas   map[string][]map[string]any
	}
	r.json(t, &exp)
	if exp.Usuario["email"] != "ana@teste.com" || len(exp.Entidades) != 2 || len(exp.Tabelas["transacoes"]) != 1 ||
		len(exp.Tabelas["contas"]) != 2 || len(exp.Tabelas["auditoria"]) == 0 {
		t.Fatalf("exportação: usuário %v, %d entidades, %d transações, %d contas", exp.Usuario, len(exp.Entidades),
			len(exp.Tabelas["transacoes"]), len(exp.Tabelas["contas"]))
	}
	temDoc := false
	for _, e := range exp.Entidades {
		temDoc = temDoc || e["documento"] == "52998224725"
	}
	if !temDoc || !bytes.Contains(r.corpo, []byte("PADARIA DA ANA")) || !bytes.Contains(r.corpo, []byte(`"casas"`)) {
		t.Fatalf("documento, transação ou casa fora da exportação: %s", r.corpo)
	}
	for _, segredo := range []string{"SEGREDO-DA-BIA", "senha_hash", "_cifrado", "token_hash", "segredo_cifrado", "bia@teste.com"} {
		if bytes.Contains(r.corpo, []byte(segredo)) {
			t.Fatalf("a exportação leva %q", segredo)
		}
	}
	r = a.exigir("POST", "/api/conta/exportar", map[string]string{"formato": "planilha"}, http.StatusOK)
	f, err := excelize.OpenReader(bytes.NewReader(r.corpo))
	if err != nil {
		t.Fatal(err)
	}
	linhas, err := f.GetRows("Transações")
	if err != nil || len(linhas) != 2 || linhas[1][2] != "PADARIA DA ANA" || linhas[1][5] != "-12.34" {
		t.Fatalf("planilha: %v %v", linhas, err)
	}
	if lista := f.GetSheetList(); strings.Join(lista, ",") != "Contas,Transações,Investimentos,Metas" {
		t.Fatalf("abas: %v", lista)
	}

	// atividade: a própria auditoria (logins e exportações), nunca a de outra pessoa
	var ativ []struct{ Acao string }
	a.exigir("GET", "/api/conta/atividade", nil, http.StatusOK).json(t, &ativ)
	acoes := map[string]int{}
	for _, e := range ativ {
		acoes[e.Acao]++
	}
	if acoes["exportacao"] != 2 || acoes["login"] == 0 && acoes["cadastro"] == 0 {
		t.Fatalf("atividade: %v", acoes)
	}
	var ativB []struct{ Acao string }
	b.exigir("GET", "/api/conta/atividade", nil, http.StatusOK).json(t, &ativB)
	for _, e := range ativB {
		if e.Acao == "exportacao" {
			t.Fatal("B vê a atividade de A")
		}
	}

	// o que a IA viu: a pergunta e as ferramentas, por pessoa
	ligarIA(t, a, 5_000_000)
	amb.claude.responde = func(map[string]any) string {
		return respostaClaude("end_turn", `{"type":"text","text":"Tudo certo."}`)
	}
	a.exigir("POST", "/api/ia/chat", map[string]any{"mensagens": []map[string]string{
		{"papel": "usuario", "texto": "Quanto gastei na padaria?"}}}, http.StatusOK)
	var envios []struct{ Tipo, Resumo string }
	a.exigir("GET", "/api/ia/envios", nil, http.StatusOK).json(t, &envios)
	if len(envios) != 1 || envios[0].Tipo != "chat" || !strings.Contains(envios[0].Resumo, "Quanto gastei na padaria?") {
		t.Fatalf("envios: %+v", envios)
	}
	var enviosB []json.RawMessage
	b.exigir("GET", "/api/ia/envios", nil, http.StatusOK).json(t, &enviosB)
	if len(enviosB) != 0 {
		t.Fatal("B vê os envios de A")
	}

	// cancelar dentro do prazo
	idB := usuarioDe(t, b)
	if _, err := amb.banco.Dono.Exec(ctx, "update usuarios set apagar_em = now() + interval '10 days' where id = $1", idB); err != nil {
		t.Fatal(err)
	}
	var sess struct {
		ApagarEm *time.Time `json:"apagar_em"`
	}
	b.exigir("GET", "/api/auth/sessao", nil, http.StatusOK).json(t, &sess)
	if sess.ApagarEm == nil {
		t.Fatal("a sessão não mostra o pedido de exclusão")
	}
	b.exigir("POST", "/api/conta/cancelar-exclusao", nil, http.StatusNoContent)
	sess.ApagarEm = nil
	b.exigir("GET", "/api/auth/sessao", nil, http.StatusOK).json(t, &sess)
	if sess.ApagarEm != nil {
		t.Fatal("cancelamento não valeu")
	}

	// apagar: senha de novo, confirmação escrita, sessões encerradas
	a.exigir("POST", "/api/conta/apagar", map[string]string{"confirmacao": "sim"}, http.StatusBadRequest)
	var ap struct {
		ApagarEm time.Time `json:"apagar_em"`
	}
	a.exigir("POST", "/api/conta/apagar", map[string]string{"confirmacao": "APAGAR"}, http.StatusOK).json(t, &ap)
	if d := time.Until(ap.ApagarEm); d < 29*24*time.Hour || d > 31*24*time.Hour {
		t.Fatalf("prazo: %v", ap.ApagarEm)
	}
	a.exigir("GET", "/api/contas", nil, http.StatusUnauthorized)

	// antes do prazo nada some; depois, o worker apaga e a casa passa para a Bia
	if err := worker.ApagarContasVencidas(ctx, amb.banco.App); err != nil {
		t.Fatal(err)
	}
	var n int
	_ = amb.banco.Dono.QueryRow(ctx, "select count(*) from usuarios where email = 'ana@teste.com'").Scan(&n)
	if n != 1 {
		t.Fatal("apagou antes do prazo")
	}
	if _, err := amb.banco.Dono.Exec(ctx, "update usuarios set apagar_em = now() - interval '1 minute' where email = 'ana@teste.com'"); err != nil {
		t.Fatal(err)
	}
	if err := worker.ApagarContasVencidas(ctx, amb.banco.App); err != nil {
		t.Fatal(err)
	}
	var usuarios, entidades, transacoes int
	_ = amb.banco.Dono.QueryRow(ctx, `select (select count(*) from usuarios where email = 'ana@teste.com'),
		(select count(*) from entidades where nome like 'Ana%'), (select count(*) from transacoes where descricao = 'PADARIA DA ANA')`).
		Scan(&usuarios, &entidades, &transacoes)
	if usuarios+entidades+transacoes != 0 {
		t.Fatalf("sobrou: %d usuários, %d entidades, %d transações", usuarios, entidades, transacoes)
	}
	var papel string
	_ = amb.banco.Dono.QueryRow(ctx, "select papel::text from membros_casa where casa_id = $1 and usuario_id = $2", casa.ID, idB).Scan(&papel)
	if papel != "dono" {
		t.Fatalf("a casa não passou para a Bia: %q", papel)
	}
	// os dados da Bia continuam
	if r := b.exigir("GET", "/api/transacoes", nil, http.StatusOK); !bytes.Contains(r.corpo, []byte("SEGREDO-DA-BIA")) {
		t.Fatal("apagou dados de outra pessoa")
	}
}

func TestTrocarSenha(t *testing.T) {
	amb := novoAmbiente(t)
	ctx := context.Background()
	a := amb.novoCliente()
	a.cadastrarCom2FA("Ana", "ana@teste.com", "")
	id := usuarioDe(t, a)
	// outra sessão da mesma pessoa (outro aparelho)
	if _, err := amb.banco.Dono.Exec(ctx, `insert into sessoes (usuario_id, token_hash, mfa_ok, expira_em)
		values ($1, '\xdeadbeef', true, now() + interval '1 day')`, id); err != nil {
		t.Fatal(err)
	}
	a.exigir("POST", "/api/auth/senha", map[string]string{"atual": "errada errada errada", "nova": "outra frase comprida"}, http.StatusUnauthorized)
	a.exigir("POST", "/api/auth/senha", map[string]string{"atual": "senha comprida de teste", "nova": "curta"}, http.StatusBadRequest)
	a.exigir("POST", "/api/auth/senha", map[string]string{"atual": "senha comprida de teste", "nova": "senha comprida de teste"}, http.StatusBadRequest)
	a.exigir("POST", "/api/auth/senha", map[string]string{"atual": "senha comprida de teste", "nova": "outra frase comprida"}, http.StatusNoContent)
	var n int
	_ = amb.banco.Dono.QueryRow(ctx, "select count(*) from sessoes where usuario_id = $1", id).Scan(&n)
	if n != 1 {
		t.Fatalf("as outras sessões continuam: %d", n)
	}
	a.exigir("POST", "/api/auth/reautenticar", map[string]string{"senha": "senha comprida de teste"}, http.StatusUnauthorized)
	a.exigir("POST", "/api/auth/reautenticar", map[string]string{"senha": "outra frase comprida"}, http.StatusNoContent)
	var ativ []struct{ Acao string }
	a.exigir("GET", "/api/conta/atividade", nil, http.StatusOK).json(t, &ativ)
	achou := false
	for _, e := range ativ {
		achou = achou || e.Acao == "senha_trocada"
	}
	if !achou {
		t.Fatalf("troca fora da auditoria: %+v", ativ)
	}
}
