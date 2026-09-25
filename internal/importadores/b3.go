package importadores

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"

	"github.com/yarion1/pi-finance/internal/core"
)

// Formatos dos arquivos da Área do Investidor da B3.
const (
	FormatoB3Negociacao   = "b3_negociacao"
	FormatoB3Movimentacao = "b3_movimentacao"
	TipoEvento            = "evento" // ajuste de quantidade (desdobro, bonificação, grupamento)
)

// ErrFormatoB3: não parece extrato de negociação nem de movimentação da B3.
var ErrFormatoB3 = errors.New("arquivo não parece da Área do Investidor da B3 (negociação ou movimentação)")

// OperacaoB3 é uma linha do extrato da B3 pronta para virar operação ou evento.
type OperacaoB3 struct {
	Data        time.Time     `json:"data"`
	Tipo        string        `json:"tipo"` // compra, venda, provento, juros, amortizacao, evento
	Codigo      string        `json:"codigo"`
	Nome        string        `json:"nome,omitempty"`
	Classe      string        `json:"classe"`
	Quantidade  core.Dec8     `json:"quantidade"` // evento: com sinal (entrada +, saída −)
	Preco       core.Dec8     `json:"preco"`
	Valor       core.Centavos `json:"valor_centavos"`
	Descricao   string        `json:"descricao"`
	Instituicao string        `json:"instituicao,omitempty"`
	Chave       string        `json:"-"`
}

// ResultadoB3 da leitura.
type ResultadoB3 struct {
	Formato string       `json:"formato"`
	Linhas  []OperacaoB3 `json:"linhas"`
	Avisos  []string     `json:"avisos,omitempty"`
}

// LerB3 lê o .xlsx (ou .csv) de negociação ou de movimentação da B3.
func LerB3(conteudo []byte) (ResultadoB3, error) {
	linhas, err := linhasPlanilha(conteudo)
	if err != nil {
		return ResultadoB3{}, err
	}
	for i, l := range linhas {
		idx := indiceColunas(l)
		switch {
		case idx.tem("data do negocio", "tipo de movimentacao", "codigo de negociacao", "quantidade", "preco"):
			return lerNegociacao(linhas[i+1:], idx), nil
		case idx.tem("entrada/saida", "data", "movimentacao", "produto", "quantidade"):
			return lerMovimentacao(linhas[i+1:], idx), nil
		}
	}
	return ResultadoB3{}, ErrFormatoB3
}

func linhasPlanilha(conteudo []byte) ([][]string, error) {
	if bytes.HasPrefix(conteudo, []byte("PK")) {
		f, err := excelize.OpenReader(bytes.NewReader(conteudo))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrFormatoB3, err)
		}
		defer f.Close()
		abas := f.GetSheetList()
		if len(abas) == 0 {
			return nil, ErrFormatoB3
		}
		return f.GetRows(abas[0])
	}
	texto := paraUTF8(conteudo)
	r := csv.NewReader(strings.NewReader(texto))
	r.Comma = ';'
	if strings.Count(strings.SplitN(texto, "\n", 2)[0], ",") > strings.Count(strings.SplitN(texto, "\n", 2)[0], ";") {
		r.Comma = ','
	}
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	linhas, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrFormatoB3, err)
	}
	return linhas, nil
}

type colunas map[string]int

func indiceColunas(cab []string) colunas {
	c := colunas{}
	for i, nome := range cab {
		c[core.NormalizarDescricao(nome)] = i
	}
	return c
}

func (c colunas) tem(nomes ...string) bool {
	for _, n := range nomes {
		if _, ok := c[n]; !ok {
			return false
		}
	}
	return true
}

func (c colunas) valor(linha []string, nome string) string {
	i, ok := c[nome]
	if !ok || i >= len(linha) {
		return ""
	}
	return strings.TrimSpace(linha[i])
}

func chaveB3(ordem map[string]int, campos ...string) string {
	base := strings.Join(campos, "|")
	ordem[base]++
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%d", base, ordem[base])))
	return "b3:" + hex.EncodeToString(h[:16])
}

func lerNegociacao(linhas [][]string, c colunas) ResultadoB3 {
	res := ResultadoB3{Formato: FormatoB3Negociacao}
	ordem := map[string]int{}
	for n, l := range linhas {
		if linhaVazia(l) {
			continue
		}
		mercado := core.NormalizarDescricao(c.valor(l, "mercado"))
		if mercado != "" && !strings.Contains(mercado, "vista") && !strings.Contains(mercado, "fracionario") {
			res.Avisos = append(res.Avisos, fmt.Sprintf("linha %d: mercado %q ignorado (opções e termo ainda não são suportados)", n+2, c.valor(l, "mercado")))
			continue
		}
		tipo := core.NormalizarDescricao(c.valor(l, "tipo de movimentacao"))
		if tipo != "compra" && tipo != "venda" {
			res.Avisos = append(res.Avisos, fmt.Sprintf("linha %d: movimentação %q ignorada", n+2, c.valor(l, "tipo de movimentacao")))
			continue
		}
		data, err1 := lerDataB3(c.valor(l, "data do negocio"))
		qtd, err2 := lerDec8B3(c.valor(l, "quantidade"))
		preco, err3 := lerDec8B3(c.valor(l, "preco"))
		codigo := codigoSemFracionario(c.valor(l, "codigo de negociacao"))
		if err1 != nil || err2 != nil || err3 != nil || codigo == "" || qtd <= 0 {
			res.Avisos = append(res.Avisos, fmt.Sprintf("linha %d: não consegui ler data, código, quantidade ou preço", n+2))
			continue
		}
		op := OperacaoB3{Data: data, Tipo: tipo, Codigo: codigo, Classe: ClasseDoCodigo(codigo, ""),
			Quantidade: qtd, Preco: preco, Descricao: strings.ToUpper(tipo[:1]) + tipo[1:], Instituicao: c.valor(l, "instituicao")}
		op.Chave = chaveB3(ordem, "neg", data.Format("2006-01-02"), tipo, codigo, qtd.String(), preco.String(), op.Instituicao)
		res.Linhas = append(res.Linhas, op)
	}
	return res
}

func lerMovimentacao(linhas [][]string, c colunas) ResultadoB3 {
	res := ResultadoB3{Formato: FormatoB3Movimentacao}
	ordem := map[string]int{}
	ignoradas := map[string]int{}
	for n, l := range linhas {
		if linhaVazia(l) {
			continue
		}
		mov := c.valor(l, "movimentacao")
		norm := core.NormalizarDescricao(mov)
		entrada := strings.HasPrefix(core.NormalizarDescricao(c.valor(l, "entrada/saida")), "credito")
		data, err := lerDataB3(c.valor(l, "data"))
		codigo, nome := separarProduto(c.valor(l, "produto"))
		if err != nil || codigo == "" {
			res.Avisos = append(res.Avisos, fmt.Sprintf("linha %d: não consegui ler a data ou o produto", n+2))
			continue
		}
		qtd, _ := lerDec8B3(c.valor(l, "quantidade"))
		preco, _ := lerDec8B3(c.valor(l, "preco unitario"))
		valor, _ := lerCentavosB3(c.valor(l, "valor da operacao"))
		op := OperacaoB3{Data: data, Codigo: codigo, Nome: nome, Classe: ClasseDoCodigo(codigo, nome),
			Descricao: mov, Instituicao: c.valor(l, "instituicao")}
		switch {
		case strings.Contains(norm, "dividendo"), strings.Contains(norm, "juros sobre capital"),
			strings.Contains(norm, "rendimento"), strings.Contains(norm, "leilao de fracao"):
			if !entrada {
				continue
			}
			op.Tipo, op.Valor = "provento", valor
		case strings.HasPrefix(norm, "amortizacao"):
			op.Tipo, op.Valor = "amortizacao", valor
		case strings.HasPrefix(norm, "juros"):
			op.Tipo, op.Valor = "juros", valor
		case strings.Contains(norm, "desdobro"), strings.Contains(norm, "bonificacao"),
			strings.Contains(norm, "grupamento"), strings.Contains(norm, "fracao em ativos"),
			norm == "atualizacao":
			if qtd == 0 {
				continue
			}
			op.Tipo, op.Quantidade, op.Preco = TipoEvento, qtd, preco
			if !entrada {
				op.Quantidade = -qtd
			}
		default:
			// compras e vendas vêm do arquivo de negociação; o resto não mexe na carteira
			ignoradas[mov]++
			continue
		}
		op.Chave = chaveB3(ordem, "mov", data.Format("2006-01-02"), norm, codigo, qtd.String(), fmt.Sprint(int64(valor)), op.Instituicao)
		res.Linhas = append(res.Linhas, op)
	}
	for mov, n := range ignoradas {
		res.Avisos = append(res.Avisos, fmt.Sprintf("%d linha(s) de %q não mexem na carteira ou vêm do arquivo de negociação", n, mov))
	}
	return res
}

func linhaVazia(l []string) bool {
	for _, v := range l {
		if strings.TrimSpace(v) != "" {
			return false
		}
	}
	return true
}

// separarProduto: "PETR4 - PETROLEO BRASILEIRO S.A. PETROBRAS" → ("PETR4", nome).
func separarProduto(p string) (string, string) {
	codigo, nome, _ := strings.Cut(p, " - ")
	return strings.ToUpper(strings.TrimSpace(codigo)), strings.TrimSpace(nome)
}

// codigoSemFracionario: PETR4F → PETR4.
func codigoSemFracionario(c string) string {
	c = strings.ToUpper(strings.TrimSpace(c))
	if len(c) > 5 && strings.HasSuffix(c, "F") && c[len(c)-2] >= '0' && c[len(c)-2] <= '9' {
		return c[:len(c)-1]
	}
	return c
}

// ClasseDoCodigo adivinha a classe pelo ticker e pelo nome do produto (a pessoa corrige
// na tela): final 34/35/32/33 é BDR; final 11 é FII ou ETF se o nome disser, senão
// unit de ação; o resto é ação.
func ClasseDoCodigo(codigo, nome string) string {
	n := core.NormalizarDescricao(nome)
	switch {
	case strings.Contains(n, "imob") || strings.Contains(n, "fii") || strings.Contains(n, "fiagro"):
		return core.ClasseFII
	case strings.Contains(n, "ishares") || strings.Contains(n, "etf") || strings.Contains(n, "indice") ||
		strings.Contains(n, "index"):
		return core.ClasseETF
	}
	for _, sufixo := range []string{"34", "35", "32", "33", "39"} {
		if strings.HasSuffix(codigo, sufixo) && len(codigo) == 6 {
			return core.ClasseBDR
		}
	}
	return core.ClasseAcao
}

func lerDataB3(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	for _, f := range []string{"02/01/2006", "2006-01-02", "01-02-06"} {
		if t, err := time.Parse(f, s); err == nil {
			return t, nil
		}
	}
	// número de série do Excel
	if n, err := strconv.ParseFloat(s, 64); err == nil && n > 20000 && n < 80000 {
		t, err := excelize.ExcelDateToTime(n, false)
		if err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), nil
		}
	}
	return time.Time{}, fmt.Errorf("data inválida: %q", s)
}

// numeroB3 normaliza "R$ 1.234,56", "1234.56" ou "-" para texto com ponto decimal.
func numeroB3(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "R$", ""), " ", ""))
	if s == "" || s == "-" {
		return "0"
	}
	if strings.Contains(s, ",") {
		s = strings.ReplaceAll(s, ".", "")
		s = strings.Replace(s, ",", ".", 1)
	}
	return s
}

func lerDec8B3(s string) (core.Dec8, error) {
	n := numeroB3(s)
	if i := strings.Index(n, "."); i >= 0 && len(n)-i-1 > 8 {
		n = n[:i+9]
	}
	return core.ParseDec8(n)
}

func lerCentavosB3(s string) (core.Centavos, error) {
	return core.ParseDecimalPonto(numeroB3(s))
}
