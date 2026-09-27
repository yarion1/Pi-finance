package api_test

import (
	"bytes"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yarion1/pi-finance/internal/financeiro"
)

type painelCasa struct {
	Mes        string `json:"mes"`
	SaldoTotal int64  `json:"saldo_total_centavos"`
	Gastos     int64  `json:"gastos_centavos"`
	PodeEditar bool   `json:"pode_editar"`
	Membros    []struct {
		UsuarioID    string  `json:"usuario_id"`
		Nome         string  `json:"nome"`
		Saldo        int64   `json:"saldo_centavos"`
		Gastos       int64   `json:"gastos_centavos"`
		Contribuicao int64   `json:"contribuicao_centavos"`
		Percentual   float64 `json:"percentual"`
	} `json:"membros"`
	Contas []struct {
		Nome, Dono, Visibilidade string
		Saldo                    int64 `json:"saldo_centavos"`
	} `json:"contas"`
	Categorias []struct {
		Nome  string
		Total int64 `json:"total_centavos"`
	} `json:"categorias"`
	Saldos []struct {
		UsuarioID string `json:"usuario_id"`
		Nome      string `json:"nome"`
		Valor     int64  `json:"valor_centavos"`
	} `json:"quem_deve_quem"`
	Despesas []struct {
		ID        string `json:"id"`
		Descricao string `json:"descricao"`
		Total     int64  `json:"total_centavos"`
		PagoNome  string `json:"pago_por_nome"`
		Partes    []struct {
			Nome     string `json:"nome"`
			Valor    int64  `json:"valor_centavos"`
			Acertada bool   `json:"acertada"`
		} `json:"partes"`
	} `json:"despesas"`
	Metas []struct {
		ID            string `json:"id"`
		Nome          string `json:"nome"`
		Atual         int64  `json:"atual_centavos"`
		PodeApagar    bool   `json:"pode_apagar"`
		Contribuicoes []struct {
			Nome  string `json:"nome"`
			Total int64  `json:"total_centavos"`
		} `json:"contribuicoes"`
	} `json:"metas"`
}

func TestCasaConsolidadaEDespesasDivididas(t *testing.T) {
	amb, a, pfA, nu, _ := prepara(t)
	hoje := financeiro.Hoje().Format("2006-01-02")
	var casa struct{ ID string }
	a.exigir("POST", "/api/casas", map[string]string{"nome": "Casa"}, http.StatusCreated).json(t, &casa)
	convidar := func(papel string) string {
		var c struct{ Link string }
		a.exigir("POST", "/api/casas/"+casa.ID+"/convites", map[string]string{"papel": papel}, http.StatusCreated).json(t, &c)
		return c.Link[strings.LastIndex(c.Link, "/")+1:]
	}
	b := amb.novoCliente()
	b.cadastrarCom2FA("Bia", "bia@teste.com", convidar("membro"))
	c := amb.novoCliente()
	c.cadastrarCom2FA("Caio", "caio@teste.com", convidar("leitor"))
	idA, idB, idC := usuarioDe(t, a), usuarioDe(t, b), usuarioDe(t, c)

	// fora da casa ninguém vê nada
	d := amb.novoCliente()
	d.cadastrarCom2FA("Davi", "davi@teste.com", convidar("membro"))
	d.exigir("DELETE", "/api/casas/"+casa.ID+"/membros/"+usuarioDe(t, d), nil, http.StatusNoContent)
	d.exigir("GET", "/api/casas/"+casa.ID+"/painel", nil, http.StatusNotFound)

	// Ana: conta privada (nu, com gasto secreto), uma compartilhada e uma "só saldo"
	var conjunta, reserva struct{ ID string }
	a.exigir("POST", "/api/contas", map[string]any{"entidade_id": pfA, "nome": "Conta da casa", "tipo": "corrente",
		"visibilidade": "compartilhada", "casa_id": casa.ID, "saldo_inicial_centavos": 200000}, http.StatusCreated).json(t, &conjunta)
	a.exigir("POST", "/api/contas", map[string]any{"entidade_id": pfA, "nome": "Reserva", "tipo": "poupanca",
		"visibilidade": "saldo", "casa_id": casa.ID, "saldo_inicial_centavos": 1000000}, http.StatusCreated).json(t, &reserva)
	a.exigir("POST", "/api/transacoes", map[string]any{"conta_id": conjunta.ID, "data": hoje, "descricao": "MERCADO DA CASA",
		"valor_centavos": -30000}, http.StatusCreated)
	a.exigir("POST", "/api/transacoes", map[string]any{"conta_id": reserva.ID, "data": hoje, "descricao": "RESERVA-SECRETA",
		"valor_centavos": -777}, http.StatusCreated)
	var jantar struct{ ID string }
	a.exigir("POST", "/api/transacoes", map[string]any{"conta_id": nu, "data": hoje, "descricao": "JANTAR-PRIVADO",
		"valor_centavos": -9001}, http.StatusCreated).json(t, &jantar)

	// Bia: a própria PF e uma conta compartilhada com gasto
	var pfB, contaB struct{ ID string }
	b.exigir("POST", "/api/entidades", map[string]any{"tipo": "PF", "nome": "Bia PF"}, http.StatusCreated).json(t, &pfB)
	b.exigir("POST", "/api/contas", map[string]any{"entidade_id": pfB.ID, "nome": "Conta da Bia", "tipo": "corrente",
		"visibilidade": "compartilhada", "casa_id": casa.ID, "saldo_inicial_centavos": 50000}, http.StatusCreated).json(t, &contaB)
	var luz struct{ ID string }
	b.exigir("POST", "/api/transacoes", map[string]any{"conta_id": contaB.ID, "data": hoje, "descricao": "LUZ",
		"valor_centavos": -10000}, http.StatusCreated).json(t, &luz)
	idLuz := luz.ID

	var p painelCasa
	c.exigir("GET", "/api/casas/"+casa.ID+"/painel", nil, http.StatusOK).json(t, &p)
	if p.SaldoTotal != 200000-30000+1000000-777+50000-10000 || p.Gastos != 40000 || len(p.Contas) != 3 || p.PodeEditar {
		t.Fatalf("consolidado (leitor): %+v", p)
	}
	membro := func(p painelCasa, id string) (saldo, gastos, contrib int64) {
		for _, m := range p.Membros {
			if m.UsuarioID == id {
				return m.Saldo, m.Gastos, m.Contribuicao
			}
		}
		t.Fatalf("membro %s fora do painel", id)
		return
	}
	if s, g, _ := membro(p, idA); s != 200000-30000+1000000-777 || g != 30000 {
		t.Fatalf("Ana: %d %d", s, g)
	}
	if s, g, _ := membro(p, idB); s != 40000 || g != 10000 {
		t.Fatalf("Bia: %d %d", s, g)
	}
	// "só saldo" soma o saldo, mas as transações não aparecem nem entram nos gastos
	r := c.exigir("GET", "/api/casas/"+casa.ID+"/painel", nil, http.StatusOK)
	for _, segredo := range []string{"RESERVA-SECRETA", "JANTAR-PRIVADO", "Nubank"} {
		if bytes.Contains(r.corpo, []byte(segredo)) {
			t.Fatalf("o painel mostra %s", segredo)
		}
	}

	// dividir: só gasto próprio, só com donos e membros, as partes fecham com o total
	dividir := func(cl *cliente, transacao string, corpo map[string]any, status int) resposta {
		corpo["transacao_id"] = transacao
		return cl.exigir("POST", "/api/casas/"+casa.ID+"/divisoes", corpo, status)
	}
	igual := map[string]any{"modo": "igual", "partes": []map[string]any{{"usuario_id": idA}, {"usuario_id": idB}}}
	dividir(b, jantar.ID, map[string]any{"modo": "igual", "partes": []map[string]any{{"usuario_id": idA}, {"usuario_id": idB}}}, http.StatusNotFound)
	dividir(a, jantar.ID, map[string]any{"modo": "igual", "partes": []map[string]any{{"usuario_id": idA}, {"usuario_id": idC}}}, http.StatusBadRequest)
	dividir(a, jantar.ID, map[string]any{"modo": "valor", "partes": []map[string]any{{"usuario_id": idA, "valor_centavos": 5000},
		{"usuario_id": idB, "valor_centavos": 1000}}}, http.StatusBadRequest)
	dividir(a, jantar.ID, map[string]any{"modo": "sorteio", "partes": []map[string]any{}}, http.StatusBadRequest)
	dividir(a, jantar.ID, igual, http.StatusCreated) // 90,01: 45,01 da Ana e 45,00 da Bia
	dividir(a, jantar.ID, igual, http.StatusBadRequest)
	// Bia divide a luz 60/40 com a Ana
	dividir(b, idLuz, map[string]any{"modo": "percentual", "partes": []map[string]any{{"usuario_id": idB, "percentual_centesimos": 6000},
		{"usuario_id": idA, "percentual_centesimos": 4000}}}, http.StatusCreated)
	// o leitor não divide
	c.exigir("POST", "/api/casas/"+casa.ID+"/divisoes", map[string]any{"transacao_id": idLuz, "modo": "igual",
		"partes": []map[string]any{{"usuario_id": idA}, {"usuario_id": idB}}}, http.StatusNotFound)

	// Ana: Bia deve 45,00 e Ana deve 40,00 → Bia deve 5,00
	a.exigir("GET", "/api/casas/"+casa.ID+"/painel", nil, http.StatusOK).json(t, &p)
	if len(p.Saldos) != 1 || p.Saldos[0].UsuarioID != idB || p.Saldos[0].Valor != 500 || len(p.Despesas) != 2 {
		t.Fatalf("quem deve quem (Ana): %+v %d despesas", p.Saldos, len(p.Despesas))
	}
	for _, dsp := range p.Despesas {
		if dsp.Descricao == "JANTAR-PRIVADO" && (dsp.Total != 9001 || len(dsp.Partes) != 2 || dsp.PagoNome != "Ana") {
			t.Fatalf("despesa: %+v", dsp)
		}
	}
	// contribuição: o jantar foi pago fora das contas compartilhadas e entra na da Ana
	if _, _, contrib := membro(p, idA); contrib != 30000+9001 {
		t.Fatalf("contribuição da Ana: %d", contrib)
	}
	if _, _, contrib := membro(p, idB); contrib != 10000 {
		t.Fatalf("contribuição da Bia (a luz já está na conta compartilhada): %d", contrib)
	}
	var pb painelCasa
	b.exigir("GET", "/api/casas/"+casa.ID+"/painel", nil, http.StatusOK).json(t, &pb)
	if len(pb.Saldos) != 1 || pb.Saldos[0].UsuarioID != idA || pb.Saldos[0].Valor != -500 {
		t.Fatalf("quem deve quem (Bia): %+v", pb.Saldos)
	}
	// o leitor não participa: não vê as despesas de ninguém
	var pc painelCasa
	c.exigir("GET", "/api/casas/"+casa.ID+"/painel", nil, http.StatusOK).json(t, &pc)
	if len(pc.Despesas) != 0 || len(pc.Saldos) != 0 {
		t.Fatalf("leitor vê despesas: %+v", pc.Despesas)
	}

	// acerto: Bia paga 5,00 à Ana; tudo em aberto entre as duas fica acertado
	var ac struct {
		De    string `json:"de_usuario"`
		Para  string `json:"para_usuario"`
		Valor int64  `json:"valor_centavos"`
	}
	c.exigir("POST", "/api/casas/"+casa.ID+"/acertos", map[string]string{"usuario_id": idA}, http.StatusForbidden)
	b.exigir("POST", "/api/casas/"+casa.ID+"/acertos", map[string]string{"usuario_id": idA}, http.StatusOK).json(t, &ac)
	if ac.De != idB || ac.Para != idA || ac.Valor != 500 {
		t.Fatalf("acerto: %+v", ac)
	}
	a.exigir("GET", "/api/casas/"+casa.ID+"/painel", nil, http.StatusOK).json(t, &p)
	if len(p.Saldos) != 0 {
		t.Fatalf("depois do acerto: %+v", p.Saldos)
	}
	for _, dsp := range p.Despesas {
		for _, pt := range dsp.Partes {
			if !pt.Acertada {
				t.Fatalf("parte em aberto: %+v", dsp)
			}
		}
	}
	a.exigir("POST", "/api/casas/"+casa.ID+"/acertos", map[string]string{"usuario_id": idB}, http.StatusOK).json(t, &ac)
	if ac.Valor != 0 {
		t.Fatalf("nada em aberto: %+v", ac)
	}
	// só quem pagou desfaz a divisão
	for _, dsp := range p.Despesas {
		if dsp.Descricao == "JANTAR-PRIVADO" {
			b.exigir("DELETE", "/api/divisoes/"+dsp.ID, nil, http.StatusNotFound)
			a.exigir("DELETE", "/api/divisoes/"+dsp.ID, nil, http.StatusNoContent)
		}
	}
	a.exigir("GET", "/api/casas/"+casa.ID+"/painel", nil, http.StatusOK).json(t, &p)
	if len(p.Despesas) != 1 || p.Despesas[0].Descricao != "LUZ" {
		t.Fatalf("depois de apagar: %+v", p.Despesas)
	}

	// metas conjuntas: todos da casa veem; donos e membros contribuem
	var meta struct{ ID string }
	a.exigir("POST", "/api/casas/"+casa.ID+"/metas", map[string]any{"nome": "Viagem da família", "tipo": "viagem",
		"alvo_centavos": 1000000, "data_alvo": time.Now().AddDate(1, 0, 0).Format("2006-01-02")}, http.StatusCreated).json(t, &meta)
	a.exigir("POST", "/api/casas/"+casa.ID+"/metas", map[string]any{"nome": "X", "tipo": "festa", "alvo_centavos": 1}, http.StatusBadRequest)
	c.exigir("POST", "/api/casas/"+casa.ID+"/metas", map[string]any{"nome": "Do leitor", "alvo_centavos": 100}, http.StatusForbidden)
	d.exigir("POST", "/api/casas/"+casa.ID+"/metas", map[string]any{"nome": "De fora", "alvo_centavos": 100}, http.StatusForbidden)
	a.exigir("POST", "/api/metas/"+meta.ID+"/contribuicoes", map[string]any{"valor_centavos": 30000}, http.StatusCreated)
	var contribB struct{ ID string }
	b.exigir("POST", "/api/metas/"+meta.ID+"/contribuicoes", map[string]any{"valor_centavos": 20000, "data": hoje}, http.StatusCreated).json(t, &contribB)
	b.exigir("POST", "/api/metas/"+meta.ID+"/contribuicoes", map[string]any{"valor_centavos": 5000}, http.StatusCreated)
	c.exigir("POST", "/api/metas/"+meta.ID+"/contribuicoes", map[string]any{"valor_centavos": 100}, http.StatusForbidden)
	d.exigir("POST", "/api/metas/"+meta.ID+"/contribuicoes", map[string]any{"valor_centavos": 100}, http.StatusForbidden)
	a.exigir("POST", "/api/metas/"+meta.ID+"/contribuicoes", map[string]any{"valor_centavos": 0}, http.StatusBadRequest)
	a.exigir("DELETE", "/api/contribuicoes/"+contribB.ID, nil, http.StatusNotFound) // a da Bia, só ela apaga
	c.exigir("GET", "/api/casas/"+casa.ID+"/painel", nil, http.StatusOK).json(t, &pc)
	if len(pc.Metas) != 1 || pc.Metas[0].Atual != 55000 || len(pc.Metas[0].Contribuicoes) != 2 || pc.Metas[0].PodeApagar ||
		pc.Metas[0].Contribuicoes[0].Nome != "Ana" || pc.Metas[0].Contribuicoes[1].Total != 25000 {
		t.Fatalf("meta conjunta: %+v", pc.Metas)
	}
	// a meta da casa não aparece em Metas (a lista da entidade)
	if r := a.exigir("GET", "/api/metas", nil, http.StatusOK); bytes.Contains(r.corpo, []byte("Viagem da família")) {
		t.Fatal("meta da casa na lista pessoal")
	}
	b.exigir("DELETE", "/api/metas/"+meta.ID, nil, http.StatusNotFound)
	a.exigir("DELETE", "/api/metas/"+meta.ID, nil, http.StatusNoContent)
}
