package importadores

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/yarion1/pi-finance/internal/core"
)

func TestLinhasDaFatura(t *testing.T) {
	r, err := LinhasDaFatura(json.RawMessage(`{"vencimento":"2026-10-10","total":"1350.00","lancamentos":[
		{"data":"2026-09-02","descricao":"MERCADO LIVRE","valor":"300.00","parcela":"2/6"},
		{"data":"2026-09-03","descricao":"LOJA - Parcela 1/3","valor":"100.00","parcela":"1/3"},
		{"data":"2026-09-05","descricao":"ESTORNO LOJA","valor":"-50.00"},
		{"data":"ontem","descricao":"X","valor":"1.00"},
		{"data":"2026-09-06","descricao":"ZERO","valor":"0"}]}`))
	if err != nil || r.Formato != FormatoDocumento || len(r.Linhas) != 3 || len(r.Avisos) != 2 {
		t.Fatalf("%+v %v", r, err)
	}
	if r.Linhas[0].Descricao != "MERCADO LIVRE - Parcela 2/6" || r.Linhas[0].Valor != -30000 ||
		r.Linhas[1].Descricao != "LOJA - Parcela 1/3" || r.Linhas[2].Valor != 5000 {
		t.Fatalf("linhas: %+v", r.Linhas)
	}
	for _, j := range []string{`x`, `{"lancamentos":[]}`} {
		if _, err := LinhasDaFatura(json.RawMessage(j)); !errors.Is(err, ErrExtraido) {
			t.Fatalf("%s: %v", j, err)
		}
	}
}

func TestLinhaDoComprovante(t *testing.T) {
	r, err := LinhaDoComprovante(json.RawMessage(`{"data":"2026-09-20","descricao":"Padaria","valor":"R$ 12.50","sentido":"saida"}`))
	if err != nil || len(r.Linhas) != 1 || r.Linhas[0].Valor != -1250 {
		t.Fatalf("%+v %v", r, err)
	}
	r, _ = LinhaDoComprovante(json.RawMessage(`{"data":"2026-09-20","descricao":"Pix de Ana","valor":"100","sentido":"entrada"}`))
	if r.Linhas[0].Valor != 10000 {
		t.Fatalf("entrada: %+v", r)
	}
	for _, j := range []string{`x`, `{"data":"2026-09-20","descricao":"a","valor":"1","sentido":"?"}`,
		`{"data":"2026-09-20","descricao":"","valor":"1","sentido":"saida"}`, `{"data":"x","descricao":"a","valor":"1","sentido":"saida"}`} {
		if _, err := LinhaDoComprovante(json.RawMessage(j)); !errors.Is(err, ErrExtraido) {
			t.Fatalf("%s: %v", j, err)
		}
	}
}

func TestOperacoesDaNota(t *testing.T) {
	r, err := OperacoesDaNota(json.RawMessage(`{"data_pregao":"2026-09-15","corretora":"XP","taxas_total":"3.00","operacoes":[
		{"codigo":"PETR4F","tipo":"C","quantidade":"10","preco":"30.00"},
		{"codigo":"ITSA4","tipo":"V","quantidade":"100","preco":"6.00"},
		{"codigo":"","tipo":"C","quantidade":"1","preco":"1"}]}`))
	if err != nil || r.Formato != FormatoNotaCorretagem || len(r.Linhas) != 2 || len(r.Avisos) != 1 {
		t.Fatalf("%+v %v", r, err)
	}
	p, i := r.Linhas[0], r.Linhas[1]
	if p.Codigo != "PETR4" || p.Tipo != "compra" || p.Classe != "acao" || i.Tipo != "venda" ||
		p.Taxas+i.Taxas != 300 || p.Taxas != 100 || p.Chave == "" || p.Chave == i.Chave || p.Instituicao != "XP" {
		t.Fatalf("operações: %+v", r.Linhas) // R$ 300 e R$ 600: 1/3 e 2/3 das taxas
	}
	q, _ := core.ParseDec8("10")
	if p.Quantidade != q {
		t.Fatalf("quantidade: %v", p.Quantidade)
	}
	for _, j := range []string{`x`, `{"data_pregao":"x","operacoes":[]}`, `{"data_pregao":"2026-09-15","operacoes":[]}`} {
		if _, err := OperacoesDaNota(json.RawMessage(j)); !errors.Is(err, ErrExtraido) {
			t.Fatalf("%s: %v", j, err)
		}
	}
}

func TestHoleriteDoDocumento(t *testing.T) {
	h, err := HoleriteDoDocumento(json.RawMessage(`{"competencia":"2026-08","empregador":"ACME","bruto":"5000.00",
		"inss":"-501.51","irrf":"","outros_descontos":"x","liquido":"4498.49"}`))
	if err != nil || h.Bruto != 500000 || h.INSS != 50151 || h.IRRF != 0 || h.Outros != 0 || h.Liquido != 449849 || h.Empregador != "ACME" {
		t.Fatalf("%+v %v", h, err)
	}
	for _, j := range []string{`x`, `{"competencia":"agosto","bruto":"1","liquido":"1"}`,
		`{"competencia":"2026-08","bruto":"","liquido":"1"}`, `{"competencia":"2026-08","bruto":"1","liquido":"0"}`} {
		if _, err := HoleriteDoDocumento(json.RawMessage(j)); !errors.Is(err, ErrExtraido) {
			t.Fatalf("%s: %v", j, err)
		}
	}
}
