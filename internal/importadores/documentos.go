package importadores

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/yarion1/pi-finance/internal/core"
)

// Documentos lidos pela IA (fatura em PDF, nota de corretagem, comprovante, holerite)
// viram as mesmas estruturas das importações de arquivo, para passar pela prévia, pela
// deduplicação e pela categorização de sempre.

// Formatos dos documentos lidos pela IA.
const (
	FormatoDocumento      = "documento"
	FormatoNotaCorretagem = "nota_corretagem"
)

// ErrExtraido: o JSON extraído não tem o mínimo para virar lançamento.
var ErrExtraido = errors.New("não consegui aproveitar o que foi lido do documento")

func valorDoc(s string) (core.Centavos, error) {
	s = strings.TrimSpace(strings.ReplaceAll(s, "R$", ""))
	if s == "" {
		return 0, errors.New("vazio")
	}
	return core.ParseDecimalPonto(s)
}

func dataDoc(s string) (time.Time, error) { return time.Parse("2006-01-02", strings.TrimSpace(s)) }

// FaturaLida: o que a IA extrai de uma fatura de cartão.
type FaturaLida struct {
	Vencimento  string `json:"vencimento"`
	Total       string `json:"total"`
	Lancamentos []struct {
		Data      string `json:"data"`
		Descricao string `json:"descricao"`
		Valor     string `json:"valor"`
		Parcela   string `json:"parcela"`
	} `json:"lancamentos"`
}

// LinhasDaFatura: compras viram saídas; estornos e créditos, entradas. A parcela vai para
// a descrição ("- Parcela 3/10") para o cartão projetar as próximas.
func LinhasDaFatura(extraido json.RawMessage) (Resultado, error) {
	var f FaturaLida
	res := Resultado{Formato: FormatoDocumento, Moeda: "BRL"}
	if err := json.Unmarshal(extraido, &f); err != nil {
		return res, ErrExtraido
	}
	for i, l := range f.Lancamentos {
		d, err1 := dataDoc(l.Data)
		v, err2 := valorDoc(l.Valor)
		desc := strings.TrimSpace(l.Descricao)
		if err1 != nil || err2 != nil || desc == "" || v == 0 {
			res.Avisos = append(res.Avisos, fmt.Sprintf("lançamento %d: data, descrição ou valor ilegível", i+1))
			continue
		}
		if p := strings.TrimSpace(l.Parcela); p != "" {
			if _, _, _, ja := core.ExtrairParcela(desc); !ja {
				desc += " - Parcela " + p
			}
		}
		res.Linhas = append(res.Linhas, Linha{Data: d, Descricao: desc, Valor: -v})
	}
	if len(res.Linhas) == 0 {
		return res, ErrExtraido
	}
	return res, nil
}

// ComprovanteLido: um pagamento ou recebimento.
type ComprovanteLido struct {
	Data      string `json:"data"`
	Descricao string `json:"descricao"`
	Valor     string `json:"valor"`
	Sentido   string `json:"sentido"` // saida, entrada
}

// LinhaDoComprovante: uma transação.
func LinhaDoComprovante(extraido json.RawMessage) (Resultado, error) {
	var c ComprovanteLido
	res := Resultado{Formato: FormatoDocumento, Moeda: "BRL"}
	if err := json.Unmarshal(extraido, &c); err != nil {
		return res, ErrExtraido
	}
	d, err1 := dataDoc(c.Data)
	v, err2 := valorDoc(c.Valor)
	desc := strings.TrimSpace(c.Descricao)
	if err1 != nil || err2 != nil || desc == "" || v <= 0 || (c.Sentido != "saida" && c.Sentido != "entrada") {
		return res, ErrExtraido
	}
	if c.Sentido == "saida" {
		v = -v
	}
	res.Linhas = []Linha{{Data: d, Descricao: desc, Valor: v}}
	return res, nil
}

// NotaLida: o que a IA extrai de uma nota de corretagem.
type NotaLida struct {
	DataPregao string `json:"data_pregao"`
	Corretora  string `json:"corretora"`
	TaxasTotal string `json:"taxas_total"`
	IRRF       string `json:"irrf"`
	Operacoes  []struct {
		Codigo     string `json:"codigo"`
		Tipo       string `json:"tipo"` // C, V
		Quantidade string `json:"quantidade"`
		Preco      string `json:"preco"`
	} `json:"operacoes"`
}

// OperacoesDaNota: compras e vendas do pregão, com as taxas da nota divididas pelo valor
// de cada operação (entram no custo da compra e saem do valor da venda).
func OperacoesDaNota(extraido json.RawMessage) (ResultadoB3, error) {
	var n NotaLida
	res := ResultadoB3{Formato: FormatoNotaCorretagem}
	if err := json.Unmarshal(extraido, &n); err != nil {
		return res, ErrExtraido
	}
	data, err := dataDoc(n.DataPregao)
	if err != nil {
		return res, ErrExtraido
	}
	taxas, _ := valorDoc(n.TaxasTotal)
	ordem := map[string]int{}
	var brutos []core.Centavos
	var total core.Centavos
	for i, o := range n.Operacoes {
		codigo := codigoSemFracionario(o.Codigo)
		qtd, err1 := core.ParseDec8(strings.TrimSpace(o.Quantidade))
		preco, err2 := core.ParseDec8(strings.TrimSpace(o.Preco))
		tipo := map[string]string{"C": "compra", "V": "venda"}[strings.ToUpper(strings.TrimSpace(o.Tipo))]
		if err1 != nil || err2 != nil || codigo == "" || qtd <= 0 || preco <= 0 || tipo == "" {
			res.Avisos = append(res.Avisos, fmt.Sprintf("operação %d: código, tipo, quantidade ou preço ilegível", i+1))
			continue
		}
		op := OperacaoB3{Data: data, Tipo: tipo, Codigo: codigo, Classe: ClasseDoCodigo(codigo, ""), Quantidade: qtd,
			Preco: preco, Descricao: strings.ToUpper(tipo[:1]) + tipo[1:] + " (nota de corretagem)",
			Instituicao: strings.TrimSpace(n.Corretora)}
		op.Chave = chaveB3(ordem, "nota", data.Format("2006-01-02"), tipo, codigo, qtd.String(), preco.String(), op.Instituicao)
		bruto := core.Valor(qtd, preco)
		brutos = append(brutos, bruto)
		total += bruto
		res.Linhas = append(res.Linhas, op)
	}
	if len(res.Linhas) == 0 {
		return res, ErrExtraido
	}
	// rateio das taxas pelo valor; a última leva o arredondamento
	resto := taxas
	for i := range res.Linhas {
		parte := core.Proporcao(taxas, core.Dec8(brutos[i]), core.Dec8(total))
		if i == len(res.Linhas)-1 {
			parte = resto
		}
		resto -= parte
		res.Linhas[i].Taxas = parte
	}
	return res, nil
}

// Holerite lido.
type Holerite struct {
	Competencia string        `json:"competencia"` // AAAA-MM
	Empregador  string        `json:"empregador"`
	Bruto       core.Centavos `json:"bruto_centavos"`
	INSS        core.Centavos `json:"inss_centavos"`
	IRRF        core.Centavos `json:"irrf_centavos"`
	Outros      core.Centavos `json:"outros_descontos_centavos"`
	Liquido     core.Centavos `json:"liquido_centavos"`
}

// HoleriteDoDocumento: os valores do contracheque (descontos vazios contam zero).
func HoleriteDoDocumento(extraido json.RawMessage) (Holerite, error) {
	var h struct {
		Competencia     string `json:"competencia"`
		Empregador      string `json:"empregador"`
		Bruto           string `json:"bruto"`
		INSS            string `json:"inss"`
		IRRF            string `json:"irrf"`
		OutrosDescontos string `json:"outros_descontos"`
		Liquido         string `json:"liquido"`
	}
	if err := json.Unmarshal(extraido, &h); err != nil {
		return Holerite{}, ErrExtraido
	}
	out := Holerite{Competencia: strings.TrimSpace(h.Competencia), Empregador: strings.TrimSpace(h.Empregador)}
	if _, err := time.Parse("2006-01", out.Competencia); err != nil {
		return out, ErrExtraido
	}
	var err error
	if out.Bruto, err = valorDoc(h.Bruto); err != nil || out.Bruto <= 0 {
		return out, ErrExtraido
	}
	if out.Liquido, err = valorDoc(h.Liquido); err != nil || out.Liquido <= 0 {
		return out, ErrExtraido
	}
	out.INSS, out.IRRF, out.Outros = descontoDoc(h.INSS), descontoDoc(h.IRRF), descontoDoc(h.OutrosDescontos)
	return out, nil
}

// descontoDoc: vazio ou ilegível conta zero; o sinal não importa (desconto é desconto).
func descontoDoc(s string) core.Centavos {
	v, err := valorDoc(s)
	if err != nil {
		return 0
	}
	return max(v, -v)
}
