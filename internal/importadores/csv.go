package importadores

import (
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	"github.com/yarion1/pi-finance/internal/core"
)

// Mapeamento diz como ler um CSV: o separador e o nome (ou a posição, começando
// em 1) das colunas. Salvo por usuário para reutilizar (mapeamentos_csv).
type Mapeamento struct {
	Separador        string   `json:"separador"`         // "," ";" ou tab; vazio = adivinhar
	ColunaData       string   `json:"coluna_data"`       // nome do cabeçalho ou "3"
	ColunasDescricao []string `json:"colunas_descricao"` // juntadas com " - "
	ColunaValor      string   `json:"coluna_valor"`
	ColunaID         string   `json:"coluna_id,omitempty"`
	FormatoData      string   `json:"formato_data"`   // "dd/mm/aaaa", "aaaa-mm-dd", "mm/dd/aaaa"
	Decimal          string   `json:"decimal"`        // "," ou "."
	InverterSinal    bool     `json:"inverter_sinal"` // fatura: valor positivo é gasto
}

type modelo struct {
	nome      string
	reconhece func(string) bool
	mapa      Mapeamento
}

func cabecalhoContem(texto string, colunas ...string) bool {
	for _, linha := range strings.SplitN(texto, "\n", 15) {
		n := core.NormalizarDescricao(linha)
		ok := true
		for _, c := range colunas {
			if !strings.Contains(n, c) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// Modelos prontos por banco (docs/SPEC.md §5). O genérico usa o mapeamento do usuário.
var modelos = []modelo{
	{
		nome:      FormatoNubankConta,
		reconhece: func(t string) bool { return cabecalhoContem(t, "data", "valor", "identificador", "descricao") },
		mapa: Mapeamento{Separador: ",", ColunaData: "data", ColunasDescricao: []string{"descricao"}, ColunaValor: "valor",
			ColunaID: "identificador", FormatoData: "dd/mm/aaaa", Decimal: "."},
	},
	{
		nome:      FormatoNubankCartao,
		reconhece: func(t string) bool { return cabecalhoContem(t, "date", "title", "amount") },
		mapa: Mapeamento{Separador: ",", ColunaData: "date", ColunasDescricao: []string{"title"}, ColunaValor: "amount",
			FormatoData: "aaaa-mm-dd", Decimal: ".", InverterSinal: true},
	},
	{
		nome:      FormatoInter,
		reconhece: func(t string) bool { return cabecalhoContem(t, "data lancamento", "historico", "valor") },
		mapa: Mapeamento{Separador: ";", ColunaData: "data lancamento", ColunasDescricao: []string{"historico", "descricao"},
			ColunaValor: "valor", FormatoData: "dd/mm/aaaa", Decimal: ","},
	},
}

var layoutsData = map[string]string{
	"dd/mm/aaaa": "02/01/2006",
	"aaaa-mm-dd": "2006-01-02",
	"mm/dd/aaaa": "01/02/2006",
	"dd-mm-aaaa": "02-01-2006",
	"dd/mm/aa":   "02/01/06",
}

func adivinharSeparador(texto string) string {
	amostra := strings.SplitN(texto, "\n", 20)
	melhor, maior := ",", 0
	for _, sep := range []string{";", ",", "\t"} {
		n := 0
		for _, l := range amostra {
			n += strings.Count(l, sep)
		}
		if n > maior {
			melhor, maior = sep, n
		}
	}
	return melhor
}

func registros(texto, sep string) ([][]string, error) {
	r := csv.NewReader(strings.NewReader(texto))
	r.Comma = []rune(sep)[0]
	r.LazyQuotes = true
	r.FieldsPerRecord = -1
	r.TrimLeadingSpace = true
	return r.ReadAll()
}

func naoVazias(l []string) int {
	n := 0
	for _, c := range l {
		if strings.TrimSpace(c) != "" {
			n++
		}
	}
	return n
}

// indiceColuna acha a coluna pelo nome (sem acento, sem caixa) ou pela posição "1", "2"...
func indiceColuna(cab []string, nome string) int {
	if nome == "" {
		return -1
	}
	var pos int
	if _, err := fmt.Sscanf(nome, "%d", &pos); err == nil && pos >= 1 && fmt.Sprint(pos) == strings.TrimSpace(nome) {
		return pos - 1
	}
	alvo := core.NormalizarDescricao(nome)
	for i, c := range cab {
		if core.NormalizarDescricao(c) == alvo {
			return i
		}
	}
	return -1
}

func lerCSV(texto string, m Mapeamento, formato string) (Resultado, error) {
	res := Resultado{Formato: formato, Moeda: "BRL"}
	sep := m.Separador
	if sep == "" {
		sep = adivinharSeparador(texto)
	}
	layout, ok := layoutsData[m.FormatoData]
	if !ok {
		return res, fmt.Errorf("formato de data desconhecido: %q", m.FormatoData)
	}
	linhas, err := registros(texto, sep)
	if err != nil {
		return res, fmt.Errorf("CSV inválido: %w", err)
	}

	// Com colunas por nome, o cabeçalho é a primeira linha que tem data e valor (os
	// bancos põem título e período antes). Com colunas por posição, não há cabeçalho:
	// linhas cuja data não lê (inclusive um cabeçalho) são ignoradas com aviso.
	inicio := -1
	var cab []string
	if !ehPosicao(m.ColunaData) {
		for i, l := range linhas {
			if indiceColuna(l, m.ColunaData) >= 0 && indiceColuna(l, m.ColunaValor) >= 0 {
				inicio, cab = i, l
				break
			}
		}
		if inicio < 0 {
			return res, fmt.Errorf("colunas %q e %q não encontradas no cabeçalho", m.ColunaData, m.ColunaValor)
		}
	}
	iData, iValor, iID := indiceColuna(cab, m.ColunaData), indiceColuna(cab, m.ColunaValor), indiceColuna(cab, m.ColunaID)
	var iDesc []int
	for _, c := range m.ColunasDescricao {
		if i := indiceColuna(cab, c); i >= 0 {
			iDesc = append(iDesc, i)
		}
	}
	if iData < 0 || iValor < 0 || len(iDesc) == 0 {
		return res, fmt.Errorf("mapeamento incompleto: data, descrição e valor são obrigatórios")
	}

	for n, l := range linhas[inicio+1:] {
		numero := inicio + n + 2
		if naoVazias(l) == 0 || iData >= len(l) || iValor >= len(l) {
			continue
		}
		campoData := strings.TrimSpace(l[iData])
		if campoData == "" {
			continue
		}
		data, err := time.Parse(layout, campoData)
		if err != nil {
			// rodapés ("Saldo final", totais) não têm data: ignora com aviso
			res.Avisos = append(res.Avisos, fmt.Sprintf("linha %d ignorada: data %q", numero, campoData))
			continue
		}
		var valor core.Centavos
		if m.Decimal == "," {
			valor, err = core.ParseBRL(l[iValor])
		} else {
			valor, err = core.ParseDecimalPonto(strings.TrimPrefix(strings.TrimSpace(l[iValor]), "R$"))
		}
		if err != nil {
			return res, fmt.Errorf("linha %d: valor inválido %q", numero, l[iValor])
		}
		if m.InverterSinal {
			valor = -valor
		}
		var partes []string
		for _, i := range iDesc {
			if i < len(l) && strings.TrimSpace(l[i]) != "" {
				partes = append(partes, strings.TrimSpace(l[i]))
			}
		}
		linha := Linha{Data: dia(data.Year(), data.Month(), data.Day()), Descricao: strings.Join(partes, " - "), Valor: valor}
		if iID >= 0 && iID < len(l) {
			linha.IDExterno = strings.TrimSpace(l[iID])
		}
		if linha.Descricao == "" {
			linha.Descricao = "(sem descrição)"
		}
		res.Linhas = append(res.Linhas, linha)
	}
	if len(res.Linhas) == 0 {
		return res, fmt.Errorf("nenhuma transação encontrada no arquivo")
	}
	return res, nil
}

func ehPosicao(s string) bool { return posicao(s) >= 0 }

func posicao(s string) int {
	var p int
	if _, err := fmt.Sscanf(s, "%d", &p); err == nil && fmt.Sprint(p) == strings.TrimSpace(s) && p >= 1 {
		return p - 1
	}
	return -1
}
