// Package importadores lê extratos (OFX e CSV) e devolve linhas prontas para a
// fila de importação (deduplicar, categorizar, conciliar). Não toca no banco.
package importadores

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"

	"github.com/yarion1/pi-finance/internal/core"
)

// Linha é uma transação lida do arquivo.
type Linha struct {
	Data      time.Time     `json:"data"`
	Descricao string        `json:"descricao"`
	Valor     core.Centavos `json:"valor_centavos"` // negativo = saída
	IDExterno string        `json:"id_externo,omitempty"`
	// Categoria que a fonte deu (Open Finance): id e nome, traduzidos por core.CategoriaPluggy
	CategoriaExternaID string `json:"-"`
	CategoriaExterna   string `json:"-"`
}

// Resultado da leitura de um arquivo.
type Resultado struct {
	Formato    string         `json:"formato"`
	Moeda      string         `json:"moeda"`
	Linhas     []Linha        `json:"linhas"`
	SaldoFinal *core.Centavos `json:"saldo_final_centavos,omitempty"`
	SaldoEm    *time.Time     `json:"saldo_em,omitempty"`
	Avisos     []string       `json:"avisos,omitempty"`
}

// Formatos reconhecidos.
const (
	FormatoOFX          = "ofx"
	FormatoNubankConta  = "nubank_conta"
	FormatoNubankCartao = "nubank_cartao"
	FormatoInter        = "inter"
	FormatoGenerico     = "generico"
	// FormatoPluggy: linhas vindas do Open Finance (não é arquivo; id externo é o da Pluggy).
	FormatoPluggy = "pluggy"
)

// ErrFormato indica arquivo que não é OFX nem um CSV reconhecido (o usuário
// precisa informar o mapeamento das colunas).
var ErrFormato = errors.New("formato de arquivo não reconhecido")

// Ler detecta o formato e lê o arquivo. Para CSV sem modelo conhecido, passe o
// mapeamento (m != nil); sem ele volta ErrFormato.
func Ler(conteudo []byte, m *Mapeamento) (Resultado, error) {
	texto := paraUTF8(conteudo)
	if pareceOFX(texto) {
		return lerOFX(texto)
	}
	if m != nil {
		return lerCSV(texto, *m, FormatoGenerico)
	}
	for _, p := range modelos {
		if p.reconhece(texto) {
			return lerCSV(texto, p.mapa, p.nome)
		}
	}
	return Resultado{}, ErrFormato
}

// paraUTF8 tira o BOM e converte de Windows-1252 quando o arquivo não é UTF-8
// (comum em extratos de bancos brasileiros).
func paraUTF8(b []byte) string {
	b = bytes.TrimPrefix(b, []byte{0xEF, 0xBB, 0xBF})
	if utf8.Valid(b) {
		return string(b)
	}
	s, err := charmap.Windows1252.NewDecoder().Bytes(b)
	if err != nil {
		return string(b)
	}
	return string(s)
}

func pareceOFX(s string) bool {
	inicio := strings.ToUpper(s[:min(len(s), 2000)])
	return strings.Contains(inicio, "OFXHEADER") || strings.Contains(inicio, "<OFX>")
}

func dia(a int, m time.Month, d int) time.Time { return time.Date(a, m, d, 0, 0, 0, 0, time.UTC) }

// Cabecalhos devolve as colunas da linha de cabeçalho de um CSV, para a tela de
// mapeamento sugerir as colunas.
func Cabecalhos(conteudo []byte, separador string) ([]string, error) {
	texto := paraUTF8(conteudo)
	sep := separador
	if sep == "" {
		sep = adivinharSeparador(texto)
	}
	linhas, err := registros(texto, sep)
	if err != nil {
		return nil, err
	}
	for _, l := range linhas {
		if naoVazias(l) >= 2 {
			return l, nil
		}
	}
	return nil, fmt.Errorf("sem cabeçalho")
}
