package api_test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/financeiro"
)

type faturasResp struct {
	Faturas []struct {
		Vencimento string `json:"vencimento"`
		Fechamento string `json:"fechamento"`
		Compras    int64  `json:"compras_centavos"`
		Projetado  int64  `json:"projetado_centavos"`
		Status     string `json:"status"`
		Transacoes int    `json:"transacoes"`
	} `json:"faturas"`
	Parcelas []struct {
		Parcela   int    `json:"parcela"`
		Total     int    `json:"total"`
		Projetada bool   `json:"projetada"`
		Mes       string `json:"mes"`
		Valor     int64  `json:"valor_centavos"`
	} `json:"parcelas_futuras"`
}

func novoCartao(t *testing.T, a *cliente, pf string) string {
	t.Helper()
	var c struct{ ID string }
	a.exigir("POST", "/api/contas", map[string]any{"entidade_id": pf, "nome": "Nubank cartão", "tipo": "cartao",
		"fechamento": 5, "vencimento": 12, "limite_centavos": 500000}, http.StatusCreated).json(t, &c)
	return c.ID
}

// Critério de aceite da fase 2: compra de R$ 1.200 em 12× aparece nas 12 faturas certas.
func TestCompraParceladaNasFaturasCertas(t *testing.T) {
	_, a, pf, _, _ := prepara(t)
	cartao := novoCartao(t, a, pf)
	// 20/09 é depois do fechamento (dia 5): 1ª parcela vence em 12/10, a última em 12/09 do ano seguinte
	a.exigir("POST", "/api/transacoes", map[string]any{"conta_id": cartao, "data": "2026-09-20", "descricao": "Geladeira",
		"valor_centavos": -120000, "parcelas": 12}, http.StatusCreated)

	var f faturasResp
	a.exigir("GET", "/api/contas/"+cartao+"/faturas", nil, http.StatusOK).json(t, &f)
	porVenc := map[string]int64{}
	for _, x := range f.Faturas {
		porVenc[x.Vencimento] = x.Compras
	}
	for i := range 12 {
		venc := core.DiaNoMes(2026, time.October+time.Month(i), 12).Format("2006-01-02")
		if porVenc[venc] != 10000 {
			t.Errorf("fatura %s com %d, esperado R$ 100,00", venc, porVenc[venc])
		}
	}
	var total int64
	for _, v := range porVenc {
		total += v
	}
	if total != 120000 {
		t.Fatalf("soma das faturas: %d", total)
	}

	var l listaTransacoes
	a.exigir("GET", "/api/transacoes?conta_id="+cartao+"&busca=Geladeira", nil, http.StatusOK).json(t, &l)
	if l.Total != 12 || l.Itens[0].Descricao != "Geladeira (12/12)" {
		t.Fatalf("12 parcelas com numeração: %d %+v", l.Total, l.Itens[0])
	}

	// mudar o fechamento recalcula a fatura de cada compra
	a.exigir("PATCH", "/api/contas/"+cartao, map[string]any{"nome": "Nubank cartão", "tipo": "cartao", "fechamento": 25, "vencimento": 3}, http.StatusNoContent)
	a.exigir("GET", "/api/contas/"+cartao+"/faturas", nil, http.StatusOK).json(t, &f)
	for _, x := range f.Faturas {
		if x.Compras > 0 && x.Vencimento[8:] != "03" {
			t.Errorf("depois de mudar os dias a fatura vence em %s", x.Vencimento)
		}
	}
}

// A última parcela importada ("Parcela 3/10") projeta as 7 que faltam.
func TestParcelaImportadaProjetaAsFuturas(t *testing.T) {
	_, a, pf, _, _ := prepara(t)
	cartao := novoCartao(t, a, pf)
	hoje := financeiro.Hoje()
	csv := fmt.Sprintf("date,title,amount\n%s,Loja de Móveis - Parcela 3/10,250.00\n%s,Padaria,12.00\n", hoje.Format("2006-01-02"), hoje.Format("2006-01-02"))
	a.exigir("POST", "/api/importacoes", map[string]any{"conta_id": cartao, "arquivo": "fatura.csv",
		"conteudo": base64.StdEncoding.EncodeToString([]byte(csv))}, http.StatusCreated)

	var f faturasResp
	a.exigir("GET", "/api/contas/"+cartao+"/faturas", nil, http.StatusOK).json(t, &f)
	projetadas, meses := 0, map[string]bool{}
	for _, p := range f.Parcelas {
		if p.Projetada {
			projetadas++
			meses[p.Mes] = true
			if p.Valor != -25000 || p.Total != 10 || p.Parcela < 4 {
				t.Errorf("projeção: %+v", p)
			}
		}
	}
	if projetadas != 7 || len(meses) != 7 {
		t.Fatalf("esperava 7 parcelas projetadas em 7 meses: %d em %d", projetadas, len(meses))
	}

	var ind struct {
		Comprometido []struct {
			Mes      string `json:"mes"`
			Parcelas int64  `json:"parcelas_centavos"`
		} `json:"comprometido"`
	}
	a.exigir("GET", "/api/indicadores", nil, http.StatusOK).json(t, &ind)
	var parcelas int64
	for _, m := range ind.Comprometido {
		parcelas += m.Parcelas
	}
	// a parcela 3 (fatura em aberto) + as 7 projetadas
	if parcelas < 7*25000 {
		t.Fatalf("comprometido com parcelas: %d", parcelas)
	}
}

type orcamentoResp struct {
	Modo      string `json:"modo"`
	Dia       int    `json:"dia"`
	DiasNoMes int    `json:"dias_no_mes"`
	Itens     []struct {
		CategoriaID   string `json:"categoria_id"`
		Limite        int64  `json:"limite_centavos"`
		SobraAnterior int64  `json:"sobra_anterior_centavos"`
		Disponivel    int64  `json:"disponivel_centavos"`
		Gasto         int64  `json:"gasto_centavos"`
		Ritmo         int64  `json:"ritmo_ideal_centavos"`
		Situacao      string `json:"situacao"`
	} `json:"itens"`
	SemOrcamento []struct {
		Nome  string `json:"nome"`
		Gasto int64  `json:"gasto_centavos"`
	} `json:"sem_orcamento"`
	Totais struct {
		Gasto int64 `json:"gasto_centavos"`
	} `json:"totais"`
	Regra *struct {
		Renda        int64 `json:"renda_centavos"`
		Necessidades struct {
			Valor int64 `json:"valor_centavos"`
		} `json:"necessidades"`
		Desejos struct {
			Valor int64 `json:"valor_centavos"`
		} `json:"desejos"`
	} `json:"regra_50_30_20"`
}

// Critério de aceite da fase 2: o orçamento mostra o ritmo do mês.
func TestOrcamentoMostraORitmo(t *testing.T) {
	_, a, pf, nu, _ := prepara(t)
	var cats []struct{ ID, Nome string }
	a.exigir("GET", "/api/categorias", nil, http.StatusOK).json(t, &cats)
	id := map[string]string{}
	for _, c := range cats {
		id[c.Nome] = c.ID
	}
	hoje := financeiro.Hoje()
	mes := hoje.Format("2006-01")
	mesPassado := core.SomarMeses(hoje, -1)

	a.exigir("PUT", "/api/orcamento", map[string]any{"entidade_id": pf, "mes": mes, "itens": []map[string]any{
		{"categoria_id": id["Alimentação"], "limite_centavos": 90000},
		{"categoria_id": id["Transporte"], "limite_centavos": 30000},
	}}, http.StatusNoContent)
	for _, tr := range []map[string]any{
		{"data": hoje.Format("2006-01-02"), "descricao": "Supermercado", "valor_centavos": -40000, "categoria_id": id["Mercado"]},
		{"data": hoje.Format("2006-01-02"), "descricao": "Cinema", "valor_centavos": -5000, "categoria_id": id["Lazer"]},
		{"data": mesPassado.Format("2006-01-02"), "descricao": "Supermercado antigo", "valor_centavos": -20000, "categoria_id": id["Mercado"]},
		{"data": hoje.Format("2006-01-02"), "descricao": "Salário", "valor_centavos": 500000, "categoria_id": id["Salário"]},
	} {
		tr["conta_id"] = nu
		a.exigir("POST", "/api/transacoes", tr, http.StatusCreated)
	}

	var o orcamentoResp
	a.exigir("GET", "/api/orcamento?mes="+mes, nil, http.StatusOK).json(t, &o)
	if o.Dia != hoje.Day() || o.DiasNoMes != core.DiasNoMes(hoje.Year(), hoje.Month()) {
		t.Fatalf("dia %d de %d", o.Dia, o.DiasNoMes)
	}
	var alim *struct {
		CategoriaID   string `json:"categoria_id"`
		Limite        int64  `json:"limite_centavos"`
		SobraAnterior int64  `json:"sobra_anterior_centavos"`
		Disponivel    int64  `json:"disponivel_centavos"`
		Gasto         int64  `json:"gasto_centavos"`
		Ritmo         int64  `json:"ritmo_ideal_centavos"`
		Situacao      string `json:"situacao"`
	}
	for i := range o.Itens {
		if o.Itens[i].CategoriaID == id["Alimentação"] {
			alim = &o.Itens[i]
		}
	}
	quer := int64(core.RitmoIdeal(90000, hoje.Day(), o.DiasNoMes))
	if alim == nil || alim.Gasto != 40000 || alim.Ritmo != quer {
		t.Fatalf("alimentação (o mercado conta na mãe): %+v, ritmo esperado %d", alim, quer)
	}
	if want := core.SituacaoOrcamento(90000, 40000, core.Centavos(quer)); alim.Situacao != want {
		t.Fatalf("situação %s, esperado %s", alim.Situacao, want)
	}
	if len(o.SemOrcamento) != 1 || o.SemOrcamento[0].Nome != "Lazer" || o.Totais.Gasto != 45000 {
		t.Fatalf("lazer sem orçamento e total: %+v %d", o.SemOrcamento, o.Totais.Gasto)
	}

	// envelopes: o que sobrou no mês passado (R$ 500 − R$ 200) passa para este
	a.exigir("PUT", "/api/orcamento", map[string]any{"entidade_id": pf, "mes": mesPassado.Format("2006-01"), "itens": []map[string]any{
		{"categoria_id": id["Alimentação"], "limite_centavos": 50000},
	}}, http.StatusNoContent)
	a.exigir("PUT", "/api/orcamento/config", map[string]any{"entidade_id": pf, "modo": "envelopes"}, http.StatusNoContent)
	a.exigir("GET", "/api/orcamento?mes="+mes, nil, http.StatusOK).json(t, &o)
	for _, it := range o.Itens {
		if it.CategoriaID == id["Alimentação"] && (it.SobraAnterior != 30000 || it.Disponivel != 120000) {
			t.Fatalf("envelope: %+v", it)
		}
	}

	// 50/30/20: mercado é necessidade, cinema é desejo
	a.exigir("PUT", "/api/orcamento/config", map[string]any{"entidade_id": pf, "modo": "50_30_20"}, http.StatusNoContent)
	a.exigir("GET", "/api/orcamento?mes="+mes, nil, http.StatusOK).json(t, &o)
	if o.Regra == nil || o.Regra.Renda != 500000 || o.Regra.Necessidades.Valor != 40000 || o.Regra.Desejos.Valor != 5000 {
		t.Fatalf("50/30/20: %+v", o.Regra)
	}

	// copiar o orçamento para o mês seguinte
	var cp struct{ Copiados int }
	a.exigir("POST", "/api/orcamento/copiar", map[string]any{"entidade_id": pf, "de": mes, "para": core.SomarMeses(hoje, 1).Format("2006-01")}, http.StatusOK).json(t, &cp)
	if cp.Copiados != 2 {
		t.Fatalf("copiados: %d", cp.Copiados)
	}
}

func TestRecorrenciaDetectadaEAlertaDeAumento(t *testing.T) {
	_, a, pf, nu, _ := prepara(t)
	hoje := financeiro.Hoje()
	for i, v := range []int64{-3990, -3990, -3990, -4490} {
		d := core.DiaNoMes(hoje.Year(), hoje.Month()-time.Month(4-i), 12)
		a.exigir("POST", "/api/transacoes", map[string]any{"conta_id": nu, "data": d.Format("2006-01-02"),
			"descricao": fmt.Sprintf("NETFLIX.COM %d", 1000+i), "valor_centavos": v}, http.StatusCreated)
	}
	var det struct{ Novas int }
	a.exigir("POST", "/api/recorrencias/detectar", map[string]any{"entidade_id": pf}, http.StatusOK).json(t, &det)
	if det.Novas != 1 {
		t.Fatalf("esperava 1 recorrência nova: %d", det.Novas)
	}
	var recs []struct {
		Descricao  string `json:"descricao"`
		Frequencia string `json:"frequencia"`
		Dia        int    `json:"dia"`
		Valor      int64  `json:"valor_centavos"`
		Subiu      bool   `json:"subiu"`
		Detectada  bool   `json:"detectada"`
		Proxima    string `json:"proxima"`
	}
	a.exigir("GET", "/api/recorrencias", nil, http.StatusOK).json(t, &recs)
	if len(recs) != 1 || recs[0].Frequencia != "mensal" || recs[0].Dia != 12 || recs[0].Valor != -4490 || !recs[0].Subiu || !recs[0].Detectada || recs[0].Proxima == "" {
		t.Fatalf("netflix: %+v", recs)
	}
	// detectar de novo não duplica nem repete o alerta
	a.exigir("POST", "/api/recorrencias/detectar", map[string]any{"entidade_id": pf}, http.StatusOK).json(t, &det)
	var alertas []struct{ Tipo string }
	a.exigir("GET", "/api/alertas", nil, http.StatusOK).json(t, &alertas)
	subiu := 0
	for _, al := range alertas {
		if al.Tipo == "recorrencia_subiu" {
			subiu++
		}
	}
	if det.Novas != 0 || subiu != 1 {
		t.Fatalf("segunda detecção: %d novas, %d alertas de aumento", det.Novas, subiu)
	}
}

func TestPatrimonioMetasEAgenda(t *testing.T) {
	_, a, pf, nu, inter := prepara(t) // Nubank com saldo inicial de R$ 1.000
	hoje := financeiro.Hoje()
	a.exigir("POST", "/api/bens", map[string]any{"entidade_id": pf, "nome": "Apartamento", "tipo": "imovel", "valor_centavos": 50000000}, http.StatusCreated)
	var div struct{ ID string }
	a.exigir("POST", "/api/dividas", map[string]any{"entidade_id": pf, "nome": "Financiamento", "tipo": "financiamento", "sistema": "price",
		"principal_centavos": 10000000, "taxa_mensal": "0,01", "prazo_meses": 12, "primeiro_vencimento": hoje.AddDate(0, 0, 10).Format("2006-01-02"),
		"conta_id": nu}, http.StatusCreated).json(t, &div)
	a.exigir("POST", "/api/dividas", map[string]any{"entidade_id": pf, "nome": "x", "tipo": "financiamento", "sistema": "price",
		"principal_centavos": 100, "taxa_mensal": "2", "prazo_meses": 12, "primeiro_vencimento": "2026-01-01"}, http.StatusBadRequest)

	var tab []struct {
		Prestacao int64 `json:"prestacao_centavos"`
	}
	a.exigir("GET", "/api/dividas/"+div.ID+"/tabela", nil, http.StatusOK).json(t, &tab)
	if len(tab) != 12 || tab[0].Prestacao != 888488 {
		t.Fatalf("tabela price: %d parcelas, 1ª %d", len(tab), tab[0].Prestacao)
	}

	var p struct {
		Hoje struct {
			Ativos   int64 `json:"ativos_centavos"`
			Passivos int64 `json:"passivos_centavos"`
			Liquido  int64 `json:"liquido_centavos"`
		} `json:"hoje"`
		Serie   []struct{} `json:"serie"`
		Dividas []struct {
			SaldoDevedor      int64 `json:"saldo_devedor_centavos"`
			ParcelasRestantes int   `json:"parcelas_restantes"`
		} `json:"dividas"`
	}
	a.exigir("GET", "/api/patrimonio", nil, http.StatusOK).json(t, &p)
	if p.Hoje.Ativos != 100000+50000000 || p.Hoje.Passivos != 10000000 || p.Hoje.Liquido != 100000+50000000-10000000 || len(p.Serie) != 13 {
		t.Fatalf("patrimônio: %+v, série %d", p.Hoje, len(p.Serie))
	}
	if p.Dividas[0].SaldoDevedor != 10000000 || p.Dividas[0].ParcelasRestantes != 12 {
		t.Fatalf("dívida: %+v", p.Dividas[0])
	}

	// meta de reserva vinculada ao Inter
	a.exigir("POST", "/api/transacoes", map[string]any{"conta_id": inter, "data": hoje.Format("2006-01-02"), "descricao": "Aporte reserva", "valor_centavos": 300000}, http.StatusCreated)
	alvo := core.SomarMeses(hoje, 11).Format("2006-01-02")
	a.exigir("POST", "/api/metas", map[string]any{"entidade_id": pf, "nome": "Reserva de emergência", "tipo": "reserva",
		"alvo_centavos": 1500000, "data_alvo": alvo, "contas_vinculadas": []string{inter}, "aporte_planejado_centavos": 100000}, http.StatusCreated)
	var metas struct {
		Metas []struct {
			Atual            int64   `json:"atual_centavos"`
			AporteNecessario int64   `json:"aporte_necessario_centavos"`
			MesesRestantes   int     `json:"meses_restantes"`
			DataPrevista     *string `json:"data_prevista"`
		} `json:"metas"`
	}
	a.exigir("GET", "/api/metas", nil, http.StatusOK).json(t, &metas)
	m := metas.Metas[0]
	if m.Atual != 300000 || m.MesesRestantes != 12 || m.AporteNecessario != 100000 || m.DataPrevista == nil {
		t.Fatalf("meta: %+v", m)
	}

	// agenda: compromisso a pagar e salário recorrente mexem no saldo projetado
	a.exigir("POST", "/api/compromissos", map[string]any{"entidade_id": pf, "descricao": "IPVA", "valor_centavos": -30000,
		"vencimento": hoje.AddDate(0, 0, 3).Format("2006-01-02"), "conta_id": nu}, http.StatusCreated)
	a.exigir("POST", "/api/recorrencias", map[string]any{"entidade_id": pf, "descricao": "Salário", "valor_centavos": 500000,
		"frequencia": "mensal", "proxima": hoje.AddDate(0, 0, 5).Format("2006-01-02"), "tipo": "receita", "conta_id": nu}, http.StatusCreated)
	var ag struct {
		Itens []struct {
			Origem string `json:"origem"`
			Valor  int64  `json:"valor_centavos"`
		} `json:"itens"`
		SaldoInicial int64      `json:"saldo_inicial_centavos"`
		SaldoFinal   int64      `json:"saldo_final_centavos"`
		Saldos       []struct{} `json:"saldos"`
	}
	a.exigir("GET", "/api/agenda?dias=30", nil, http.StatusOK).json(t, &ag)
	origens := map[string]int{}
	var soma int64
	for _, it := range ag.Itens {
		origens[it.Origem]++
		soma += it.Valor
	}
	if origens["compromisso"] != 1 || origens["recorrencia"] < 1 || origens["divida"] < 1 || len(ag.Saldos) != 31 {
		t.Fatalf("agenda: %+v", origens)
	}
	if ag.SaldoInicial != 100000+300000 || ag.SaldoFinal != ag.SaldoInicial+soma {
		t.Fatalf("saldo projetado: inicial %d, final %d, itens %d", ag.SaldoInicial, ag.SaldoFinal, soma)
	}
}
