package openfinance

import (
	"encoding/json"
	"testing"

	"github.com/yarion1/pi-finance/internal/core"
	"github.com/yarion1/pi-finance/internal/pluggy"
)

func TestOperacaoDoMovimento(t *testing.T) {
	texto := func(s string) *string { return &s }
	casos := []struct {
		nome      string
		m         pluggy.MovimentoInvestimento
		ok        bool
		tipo      string
		qtd, prec string
	}{
		{"aporte sem quantidade vira 1 × valor",
			pluggy.MovimentoInvestimento{ID: "a", MovementType: "CREDIT", Type: texto("BUY"), Amount: "1000.50", Date: "2026-01-10"},
			true, "compra", "1", "1000.5"},
		{"compra com quantidade",
			pluggy.MovimentoInvestimento{ID: "b", MovementType: "credit", Quantity: "10", Amount: "325.40", Date: "2026-01-10T13:00:00.000Z"},
			true, "compra", "10", "32.54"},
		{"resgate usa o líquido, com qualquer sinal",
			pluggy.MovimentoInvestimento{ID: "c", MovementType: "DEBIT", Type: texto("SELL"), Amount: "-1100", NetAmount: "-1080", Date: "2026-03-01"},
			true, "venda", "1", "1080"},
		{"resgate sem líquido usa o bruto",
			pluggy.MovimentoInvestimento{ID: "d", MovementType: "DEBIT", Amount: "500", Date: "2026-03-01"},
			true, "venda", "1", "500"},
		{"imposto fica de fora",
			pluggy.MovimentoInvestimento{ID: "e", MovementType: "DEBIT", Type: texto("tax"), Amount: "10", Date: "2026-03-01"}, false, "", "", ""},
		{"sentido desconhecido",
			pluggy.MovimentoInvestimento{ID: "f", MovementType: "", Amount: "10", Date: "2026-03-01"}, false, "", "", ""},
		{"data inválida",
			pluggy.MovimentoInvestimento{ID: "g", MovementType: "CREDIT", Amount: "10", Date: "ontem"}, false, "", "", ""},
		{"valor zero",
			pluggy.MovimentoInvestimento{ID: "h", MovementType: "CREDIT", Amount: "0", Date: "2026-03-01"}, false, "", "", ""},
		{"valor inválido",
			pluggy.MovimentoInvestimento{ID: "i", MovementType: "CREDIT", Amount: "1e3", Date: "2026-03-01"}, false, "", "", ""},
	}
	for _, c := range casos {
		op, ok := OperacaoDoMovimento(c.m)
		if ok != c.ok {
			t.Errorf("%s: ok = %v", c.nome, ok)
			continue
		}
		if !ok {
			continue
		}
		qtd, _ := core.ParseDec8(c.qtd)
		prec, _ := core.ParseDec8(c.prec)
		if op.Tipo != c.tipo || op.Quantidade != qtd || op.Preco != prec {
			t.Errorf("%s: %+v (quer %s %s × %s)", c.nome, op, c.tipo, c.qtd, c.prec)
		}
	}
}

func TestOpcionais(t *testing.T) {
	if centavosOpcional(json.Number("")) != nil || centavosOpcional(json.Number("x")) != nil {
		t.Fatal("vazio e inválido viram nil")
	}
	if v := centavosOpcional(json.Number("+12.34")); v == nil || *v != 1234 {
		t.Fatalf("12,34: %v", v)
	}
	ruim, boa := "x", "2026-02-03"
	if dataOpcional(nil) != nil || dataOpcional(&ruim) != nil {
		t.Fatal("data vazia e inválida viram nil")
	}
	if d := dataOpcional(&boa); d == nil || d.Format("2006-01-02") != boa {
		t.Fatalf("data: %v", d)
	}
}
