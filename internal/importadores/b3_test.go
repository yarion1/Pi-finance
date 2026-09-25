package importadores

import (
	"bytes"
	"testing"

	"github.com/xuri/excelize/v2"

	"github.com/yarion1/pi-finance/internal/core"
)

func planilha(t *testing.T, linhas [][]any) []byte {
	t.Helper()
	f := excelize.NewFile()
	for i, l := range linhas {
		celula, _ := excelize.CoordinatesToCellName(1, i+1)
		if err := f.SetSheetRow("Sheet1", celula, &l); err != nil {
			t.Fatal(err)
		}
	}
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestNegociacaoB3(t *testing.T) {
	arq := planilha(t, [][]any{
		{"Data do Negócio", "Tipo de Movimentação", "Mercado", "Prazo/Vencimento", "Instituição", "Código de Negociação", "Quantidade", "Preço", "Valor"},
		{"02/01/2026", "Compra", "Mercado à Vista", "-", "XP INVESTIMENTOS", "PETR4", 100, 38.5, 3850},
		{"02/01/2026", "Compra", "Mercado Fracionário", "-", "XP INVESTIMENTOS", "PETR4F", 5, "38,50", "192,50"},
		{"05/01/2026", "Venda", "Mercado à Vista", "-", "XP INVESTIMENTOS", "HGLG11", 10, "R$ 160,10", "1.601,00"},
		{"05/01/2026", "Compra", "Opção de Compra", "-", "XP", "PETRA400", 100, 1, 100},
		{"06/01/2026", "Compra", "Mercado à Vista", "-", "XP", "AAPL34", "abc", 1, 1},
		{"", "", "", "", "", "", "", "", ""},
		{"07/01/2026", "Transferência", "Mercado à Vista", "-", "XP", "PETR4", 1, 1, 1},
		{"08/01/2026", "Compra", "Mercado à Vista", "-", "XP", "AAPL34", 2, 50, 100},
	})
	r, err := LerB3(arq)
	if err != nil {
		t.Fatal(err)
	}
	if r.Formato != FormatoB3Negociacao || len(r.Linhas) != 4 || len(r.Avisos) != 3 {
		t.Fatalf("negociação: %+v", r)
	}
	a := r.Linhas[0]
	if a.Tipo != "compra" || a.Codigo != "PETR4" || a.Quantidade != 100*core.Um || a.Preco != 385*core.Um/10 || a.Classe != core.ClasseAcao || a.Descricao != "Compra" {
		t.Fatalf("PETR4: %+v", a)
	}
	if r.Linhas[1].Codigo != "PETR4" || r.Linhas[1].Chave == a.Chave {
		t.Fatalf("fracionário: %+v", r.Linhas[1])
	}
	if v := r.Linhas[2]; v.Tipo != "venda" || v.Preco != 16010*core.Um/100 {
		t.Fatalf("venda: %+v", v)
	}
	if r.Linhas[3].Classe != core.ClasseBDR {
		t.Fatalf("BDR: %+v", r.Linhas[3])
	}
	// o mesmo arquivo lido de novo gera as mesmas chaves (dedup)
	r2, _ := LerB3(arq)
	if r2.Linhas[0].Chave != a.Chave {
		t.Fatal("chave instável")
	}
}

func TestMovimentacaoB3(t *testing.T) {
	csv := "Entrada/Saída;Data;Movimentação;Produto;Instituição;Quantidade;Preço unitário;Valor da Operação\n" +
		"Credito;15/01/2026;Dividendo;PETR4 - PETROLEO BRASILEIRO S.A. PETROBRAS;XP;100;0,5;50,00\n" +
		"Credito;15/01/2026;Juros Sobre Capital Próprio;ITSA4 - ITAUSA S.A.;XP;100;0,1;8,50\n" +
		"Credito;20/01/2026;Rendimento;HGLG11 - CSHG LOGISTICA FDO INV IMOB;XP;10;1,10;11,00\n" +
		"Debito;20/01/2026;Rendimento;HGLG11 - CSHG LOGISTICA FDO INV IMOB;XP;10;1,10;11,00\n" +
		"Credito;22/01/2026;Desdobro;BBAS3 - BANCO DO BRASIL S.A.;XP;100;-;-\n" +
		"Debito;23/01/2026;Fração em Ativos;BBAS3 - BANCO DO BRASIL S.A.;XP;0,5;-;-\n" +
		"Credito;24/01/2026;Bonificação em Ativos;ITSA4 - ITAUSA S.A.;XP;10;2,5;-\n" +
		"Credito;25/01/2026;Bonificação em Ativos;ITSA4 - ITAUSA S.A.;XP;0;2,5;-\n" +
		"Credito;26/01/2026;Transferência - Liquidação;PETR4 - PETROLEO BRASILEIRO S.A. PETROBRAS;XP;100;38,5;3.850,00\n" +
		"Credito;27/01/2026;Amortização;CDB BANCO X;XP;0;-;100,00\n" +
		"Credito;28/01/2026;Juros;CDB BANCO X;XP;0;-;12,34\n" +
		"Credito;29/01/2026;Rendimento;BOVA11 - ISHARES BOVESPA FUNDO DE INDICE;XP;1;0;1,00\n" +
		"Credito;data ruim;Dividendo;PETR4 - PETROBRAS;XP;1;1;1\n"
	r, err := LerB3([]byte(csv))
	if err != nil {
		t.Fatal(err)
	}
	if r.Formato != FormatoB3Movimentacao || len(r.Linhas) != 9 {
		t.Fatalf("movimentação: %d %+v", len(r.Linhas), r)
	}
	div := r.Linhas[0]
	if div.Tipo != "provento" || div.Codigo != "PETR4" || div.Valor != 5000 || div.Nome != "PETROLEO BRASILEIRO S.A. PETROBRAS" {
		t.Fatalf("dividendo: %+v", div)
	}
	if r.Linhas[2].Classe != core.ClasseFII || r.Linhas[2].Valor != 1100 {
		t.Fatalf("rendimento de FII: %+v", r.Linhas[2])
	}
	if e := r.Linhas[3]; e.Tipo != TipoEvento || e.Quantidade != 100*core.Um {
		t.Fatalf("desdobro: %+v", e)
	}
	if e := r.Linhas[4]; e.Quantidade != -core.Um/2 {
		t.Fatalf("fração: %+v", e)
	}
	if e := r.Linhas[5]; e.Preco != 25*core.Um/10 {
		t.Fatalf("bonificação: %+v", e)
	}
	if r.Linhas[6].Tipo != "amortizacao" || r.Linhas[7].Tipo != "juros" || r.Linhas[7].Valor != 1234 {
		t.Fatalf("renda fixa: %+v %+v", r.Linhas[6], r.Linhas[7])
	}
	if r.Linhas[8].Classe != core.ClasseETF {
		t.Fatalf("ETF: %+v", r.Linhas[8])
	}
	if len(r.Avisos) != 2 {
		t.Fatalf("avisos: %v", r.Avisos)
	}

	// CSV com vírgula e data em número de série do Excel
	csv2 := "Entrada/Saída,Data,Movimentação,Produto,Quantidade\nCredito,46037,Desdobro,BBAS3 - BB,5\n"
	r, err = LerB3([]byte(csv2))
	if err != nil || len(r.Linhas) != 1 || r.Linhas[0].Data.Format("2006-01-02") != "2026-01-15" {
		t.Fatalf("csv com vírgula: %+v %v", r, err)
	}
}

func TestB3Invalido(t *testing.T) {
	for _, arq := range [][]byte{[]byte("a;b;c\n1;2;3\n"), []byte("PK nao e zip"), []byte("\"a\n")} {
		if _, err := LerB3(arq); err == nil {
			t.Errorf("%q deveria falhar", arq)
		}
	}
	vazio := excelize.NewFile()
	var buf bytes.Buffer
	_ = vazio.Write(&buf)
	if _, err := LerB3(buf.Bytes()); err == nil {
		t.Error("planilha sem cabeçalho deveria falhar")
	}
	if _, err := lerDataB3("99999"); err == nil {
		t.Error("número fora da faixa de datas")
	}
	if v, _ := lerDec8B3("1,123456789"); v != 112345678 {
		t.Errorf("mais de 8 casas: %v", v)
	}
	if ClasseDoCodigo("SNAG11", "SUNO AGRO FIAGRO") != core.ClasseFII || ClasseDoCodigo("TAEE11", "TAESA UNIT") != core.ClasseAcao {
		t.Error("classes")
	}
}
