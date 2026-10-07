package api_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/financeiro"
)

type resumoCartoes struct {
	Meses        []string `json:"meses"`
	TotalPorMes  []int64  `json:"total_por_mes_centavos"`
	TotalDevendo int64    `json:"total_devendo_centavos"`
	Cartoes      []struct {
		ID      string `json:"id"`
		Nome    string `json:"nome"`
		Devendo int64  `json:"devendo_centavos"`
		SemDias bool   `json:"sem_dias"`
		Faturas []struct {
			Vencimento string `json:"vencimento"`
			Valor      int64  `json:"valor_centavos"`
			Status     string `json:"status"`
			Informado  bool   `json:"informado"`
		} `json:"faturas"`
	} `json:"cartoes"`
}

func TestResumoCartoesEValorInformado(t *testing.T) {
	amb, a, pf, _, _ := prepara(t)
	ctx := context.Background()
	hoje := financeiro.Hoje()
	var picpay, ml, semDias struct{ ID string }
	a.exigir("POST", "/api/contas", map[string]any{"entidade_id": pf, "nome": "PicPay", "tipo": "cartao", "fechamento": 3,
		"vencimento": 10, "limite_centavos": 1000000}, http.StatusCreated).json(t, &picpay)
	a.exigir("POST", "/api/contas", map[string]any{"entidade_id": pf, "nome": "Mercado Pago", "tipo": "cartao", "fechamento": 25,
		"vencimento": 5}, http.StatusCreated).json(t, &ml)
	a.exigir("POST", "/api/contas", map[string]any{"entidade_id": pf, "nome": "Cartão sem dias", "tipo": "cartao"},
		http.StatusCreated).json(t, &semDias)
	vPic := core.ProximosVencimentos(hoje, 3, 10, financeiro.MesesResumoCartoes)
	vML := core.ProximosVencimentos(hoje, 25, 5, financeiro.MesesResumoCartoes)
	informar := func(conta string, venc string, valor int64, status int) {
		t.Helper()
		a.exigir("PUT", "/api/contas/"+conta+"/faturas/"+venc, map[string]any{"valor_centavos": valor}, status)
	}
	pic0, ml0, ml1 := vPic[0].Format("2006-01-02"), vML[0].Format("2006-01-02"), vML[1].Format("2006-01-02")

	informar(picpay.ID, pic0, 400000, http.StatusNoContent)
	informar(ml.ID, ml0, 500000, http.StatusNoContent)
	informar(ml.ID, ml1, 120000, http.StatusNoContent)

	var r resumoCartoes
	a.exigir("GET", "/api/cartoes/resumo", nil, http.StatusOK).json(t, &r)
	if len(r.Cartoes) != 3 || len(r.Meses) != 6 || r.TotalDevendo != 1020000 {
		t.Fatalf("resumo: %+v", r)
	}
	por := map[string]int{}
	for i, c := range r.Cartoes {
		por[c.Nome] = i
	}
	cp, cm := r.Cartoes[por["PicPay"]], r.Cartoes[por["Mercado Pago"]]
	if cp.Devendo != 400000 || cm.Devendo != 620000 || !r.Cartoes[por["Cartão sem dias"]].SemDias {
		t.Fatalf("por cartão: PicPay %d, ML %d", cp.Devendo, cm.Devendo)
	}
	if cp.Faturas[0].Vencimento != pic0 || cp.Faturas[0].Valor != 400000 || !cp.Faturas[0].Informado || cp.Faturas[1].Valor != 0 {
		t.Fatalf("faturas do PicPay: %+v", cp.Faturas)
	}
	// o total de cada mês soma os cartões que vencem nele
	var soma int64
	for i, m := range r.Meses {
		var esperado int64
		if pic0[:7] == m {
			esperado += 400000
		}
		if ml0[:7] == m {
			esperado += 500000
		}
		if ml1[:7] == m {
			esperado += 120000
		}
		if r.TotalPorMes[i] != esperado {
			t.Fatalf("mês %s: %d, esperado %d (%+v)", m, r.TotalPorMes[i], esperado, r.TotalPorMes)
		}
		soma += r.TotalPorMes[i]
	}
	if soma != 1020000 {
		t.Fatalf("soma dos meses: %d", soma)
	}

	// mudar o valor troca o ajuste (não acumula); com compras lançadas, o ajuste é a diferença
	data := core.FechamentoDaFatura(vPic[0], 3, 10).AddDate(0, 0, -2).Format("2006-01-02")
	a.exigir("POST", "/api/transacoes", map[string]any{"conta_id": picpay.ID, "data": data, "descricao": "MERCADO",
		"valor_centavos": -100000}, http.StatusCreated)
	informar(picpay.ID, pic0, 350000, http.StatusNoContent)
	informar(picpay.ID, pic0, 50000, http.StatusNoContent) // menos que as compras: vira estorno
	a.exigir("GET", "/api/cartoes/resumo", nil, http.StatusOK).json(t, &r)
	if v := r.Cartoes[por["PicPay"]].Faturas[0].Valor; v != 50000 {
		t.Fatalf("valor depois de mudar: %d", v)
	}
	var ajustes int
	_ = amb.banco.Dono.QueryRow(ctx, "select count(*) from transacoes where conta_id = $1 and descricao_original = $2",
		picpay.ID, financeiro.MarcaAjusteFatura).Scan(&ajustes)
	if ajustes != 1 {
		t.Fatalf("ajustes acumulados: %d", ajustes)
	}
	informar(picpay.ID, pic0, 100000, http.StatusNoContent) // igual às compras: o ajuste some
	_ = amb.banco.Dono.QueryRow(ctx, "select count(*) from transacoes where conta_id = $1 and descricao_original = $2",
		picpay.ID, financeiro.MarcaAjusteFatura).Scan(&ajustes)
	if ajustes != 0 {
		t.Fatalf("ajuste zerado continua: %d", ajustes)
	}

	// validações e privacidade
	outroDia := vPic[0].AddDate(0, 0, 1).Format("2006-01-02")
	informar(picpay.ID, outroDia, 1000, http.StatusBadRequest)
	informar(picpay.ID, pic0, -1, http.StatusBadRequest)
	informar(semDias.ID, pic0, 1000, http.StatusBadRequest)
	var conv struct{ Link string }
	a.exigir("POST", "/api/convites-conta", nil, http.StatusCreated).json(t, &conv)
	b := amb.novoCliente()
	b.cadastrarCom2FA("Bia", "bia@teste.com", conv.Link[strings.LastIndex(conv.Link, "/")+1:])
	b.exigir("PUT", "/api/contas/"+picpay.ID+"/faturas/"+pic0, map[string]any{"valor_centavos": 1}, http.StatusNotFound)
	var rb resumoCartoes
	b.exigir("GET", "/api/cartoes/resumo", nil, http.StatusOK).json(t, &rb)
	if len(rb.Cartoes) != 0 || rb.TotalDevendo != 0 {
		t.Fatalf("B vê os cartões de A: %+v", rb)
	}
}
