package importadores

import (
	"errors"
	"os"
	"testing"

	"github.com/yarion1/pi-finance/internal/core"
)

func ler(t *testing.T, arquivo string, m *Mapeamento) Resultado {
	t.Helper()
	b, err := os.ReadFile("testdata/" + arquivo)
	if err != nil {
		t.Fatal(err)
	}
	r, err := Ler(b, m)
	if err != nil {
		t.Fatalf("%s: %v", arquivo, err)
	}
	return r
}

type esperado struct {
	data      string
	descricao string
	valor     core.Centavos
	id        string
}

func conferir(t *testing.T, r Resultado, formato string, linhas []esperado) {
	t.Helper()
	if r.Formato != formato {
		t.Fatalf("formato %q, esperado %q", r.Formato, formato)
	}
	if len(r.Linhas) != len(linhas) {
		t.Fatalf("%d linhas, esperado %d: %+v", len(r.Linhas), len(linhas), r.Linhas)
	}
	for i, e := range linhas {
		l := r.Linhas[i]
		if l.Data.Format("2006-01-02") != e.data || l.Descricao != e.descricao || l.Valor != e.valor || l.IDExterno != e.id {
			t.Errorf("linha %d: %+v, esperado %+v", i+1, l, e)
		}
	}
}

func TestOFX(t *testing.T) {
	r := ler(t, "extrato.ofx", nil)
	conferir(t, r, FormatoOFX, []esperado{
		{"2026-09-05", "Padaria São João", -4590, "abc-001"},
		{"2026-09-10", "EMPRESA LTDA - Transferência recebida", 500000, "abc-002"},
		{"2026-09-12", "Pix enviado", -120000, "abc-003"},
	})
	if r.Moeda != "BRL" || r.SaldoFinal == nil || *r.SaldoFinal != 375410 || r.SaldoEm.Format("2006-01-02") != "2026-09-20" {
		t.Errorf("cabeçalho do OFX: %+v", r)
	}
}

func TestNubankConta(t *testing.T) {
	conferir(t, ler(t, "nubank_conta.csv", nil), FormatoNubankConta, []esperado{
		{"2026-09-01", "Compra no débito - Padaria", -4590, "66f1a2b3-0001"},
		{"2026-09-01", "Compra no débito - Padaria", -4590, "66f1a2b3-0002"},
		{"2026-09-05", "Transferência recebida pelo Pix - EMPRESA LTDA", 500000, "66f1a2b3-0003"},
	})
}

func TestNubankCartao(t *testing.T) {
	// na fatura, valor positivo é gasto
	conferir(t, ler(t, "nubank_cartao.csv", nil), FormatoNubankCartao, []esperado{
		{"2026-09-02", "Uber *Trip", -2345, ""},
		{"2026-09-03", "Pagamento recebido", 50000, ""},
	})
}

func TestInter(t *testing.T) {
	conferir(t, ler(t, "inter.csv", nil), FormatoInter, []esperado{
		{"2026-09-03", "Pix enviado - Maria Silva", -120000, ""},
		{"2026-09-04", "Pix recebido - João", 25000, ""},
	})
}

func TestGenerico(t *testing.T) {
	b, _ := os.ReadFile("testdata/generico.csv")
	if _, err := Ler(b, nil); !errors.Is(err, ErrFormato) {
		t.Fatalf("CSV desconhecido sem mapeamento deveria pedir mapeamento: %v", err)
	}
	m := &Mapeamento{ColunaData: "quando", ColunasDescricao: []string{"O que"}, ColunaValor: "Quanto", FormatoData: "dd/mm/aaaa", Decimal: ","}
	r := ler(t, "generico.csv", m)
	conferir(t, r, FormatoGenerico, []esperado{
		{"2026-09-10", "Mercado Bom Preço", -31240, ""},
		{"2026-09-11", "Salário", 800000, ""},
	})
	if len(r.Avisos) != 1 {
		t.Errorf("a linha de total deveria virar aviso: %v", r.Avisos)
	}
	cab, err := Cabecalhos(b, "")
	if err != nil || len(cab) != 3 || cab[1] != "O que" {
		t.Errorf("cabeçalhos: %v %v", cab, err)
	}

	// por posição, sem depender do cabeçalho
	pos := &Mapeamento{Separador: ";", ColunaData: "1", ColunasDescricao: []string{"2"}, ColunaValor: "3", FormatoData: "dd/mm/aaaa", Decimal: ","}
	if r := ler(t, "generico.csv", pos); len(r.Linhas) != 2 {
		t.Errorf("por posição: %+v", r.Linhas)
	}
}

func TestArquivoRuim(t *testing.T) {
	if _, err := Ler([]byte("nada a ver"), nil); !errors.Is(err, ErrFormato) {
		t.Errorf("texto qualquer: %v", err)
	}
	if _, err := Ler([]byte("<OFX><BANKTRANLIST><STMTTRN><DTPOSTED>xx<TRNAMT>1</STMTTRN></BANKTRANLIST></OFX>"), nil); err == nil {
		t.Error("OFX com data ruim deveria falhar")
	}
	m := &Mapeamento{ColunaData: "quando", ColunasDescricao: []string{"o que"}, ColunaValor: "quanto", FormatoData: "dd/mm/aaaa", Decimal: ","}
	if _, err := Ler([]byte("Quando;O que;Quanto\n10/09/2026;x;abc\n"), m); err == nil {
		t.Error("valor inválido deveria falhar")
	}
}
