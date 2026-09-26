package core

import (
	"fmt"
	"testing"
	"time"
)

func TestDetectarAlertas(t *testing.T) {
	hoje := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	d := func(n int) time.Time { return hoje.AddDate(0, 0, n) }
	tx := func(id, conta, cat, desc string, valor Centavos, dia int) TransacaoAlerta {
		return TransacaoAlerta{ID: id, Conta: conta, Categoria: cat, Descricao: desc, Valor: valor, Data: d(dia)}
	}
	var hist []TransacaoAlerta
	for i, v := range []Centavos{-8000, -9000, -10000, -11000, -12000} { // mediana R$ 100
		hist = append(hist, tx(fmt.Sprint("h", i), "c1", "mercado", "SUPERMERCADO DIA", v, -60+i))
	}
	hist = append(hist, tx("h9", "c1", "uber", "UBER *TRIP", -2000, -5)) // categoria sem amostras
	hist = append(hist, tx("netflix-1", "c2", "streaming", "NETFLIX.COM 123", -5590, -2))

	novas := []TransacaoAlerta{
		tx("n1", "c1", "mercado", "SUPERMERCADO EXTRA", -45000, -1),  // 4,5× a mediana e R$ 350 acima
		tx("n2", "c1", "mercado", "SUPERMERCADO DIA", -25000, -1),    // 2,5×: dentro do padrão
		tx("n3", "c2", "streaming", "NETFLIX.COM 456", -5590, 0),     // repetiu em 2 dias
		tx("n4", "c1", "", "TARIFA BANCARIA CESTA", -3290, -3),       // tarifa
		tx("n5", "c3", "", "IOF COMPRA INTERNACIONAL", -412, -3),     // IOF
		tx("n6", "c3", "", "JUROS ROTATIVO", -8900, -3),              // juros
		tx("n7", "c1", "uber", "UBER *TRIP", -90000, -1),             // sem padrão na categoria
		tx("n8", "c1", "mercado", "SUPERMERCADO EXTRA", -45000, -90), // antigo demais
		tx("n9", "c1", "", "RENDIMENTO JUROS", 1200, -1),             // entrada: ignora
		tx("na", "c1", "cafe", "CAFE", -900, -1),                     // pequeno
		tx("nb", "c1", "cafe", "CAFE", -900, -1),                     // igual, mas abaixo do mínimo
		tx("nc", "c4", "loja", "LOJA X PARCELA 2/10", -10000, -1),    // parcela
		tx("nd", "c4", "loja", "LOJA X PARCELA 2/10", -10000, -1),
		tx("ne", "c5", "", "PADARIA", -6000, -1), tx("nf", "c5", "", "PADARIA", -6000, -1), // mesmo dia: só uma
		tx("ng", "c5", "", "PADARIA", -6000, -9),                                   // 8 dias antes: fora da janela
		tx("nh", "c1", "", "!!!", -6000, -1), tx("ni", "c1", "", "!!!", -6000, -1), // sem descrição útil
		tx("nj", "c1", "", "FUTURA", -6000, 2), // data no futuro
	}
	novas[11].Parcela, novas[12].Parcela = true, true
	got := DetectarAlertas(novas, hist, hoje)
	quer := map[string]string{"n1": "gasto_fora_do_padrao", "n3": "cobranca_duplicada", "n4": "tarifa_bancaria",
		"n5": "juros_iof", "n6": "juros_iof", "nf": "cobranca_duplicada"}
	if len(got) != len(quer) {
		t.Fatalf("alertas: %+v", got)
	}
	for _, a := range got {
		if quer[a.Transacao.ID] != a.Tipo {
			t.Errorf("%s: %s", a.Transacao.ID, a.Tipo)
		}
		switch a.Transacao.ID {
		case "n1":
			if a.Referencia != 10000 {
				t.Errorf("mediana: %d", a.Referencia)
			}
		case "n3":
			if a.Outra != "netflix-1" {
				t.Errorf("outra: %s", a.Outra)
			}
		case "nf":
			if a.Outra != "ne" {
				t.Errorf("outra: %s", a.Outra)
			}
		}
	}
	if got[0].Transacao.ID != "n1" {
		t.Errorf("o maior primeiro: %s", got[0].Transacao.ID)
	}

	// teto por rodada
	var muitas []TransacaoAlerta
	for i := range MaxAlertasPorRodada + 5 {
		muitas = append(muitas, tx(fmt.Sprint("t", i), "c1", "", "TARIFA", Centavos(-100-i), -1))
	}
	if n := len(DetectarAlertas(muitas, nil, hoje)); n != MaxAlertasPorRodada {
		t.Fatalf("teto: %d", n)
	}
}

func TestMediana(t *testing.T) {
	if Mediana(nil) != 0 || Mediana([]Centavos{3, 1, 2}) != 2 || Mediana([]Centavos{4, 1, 3, 2}) != 2 {
		t.Fatal("mediana")
	}
}
