package importadores

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
	"time"

	"github.com/yarion1/pi-finance/internal/core"
)

// NFS-e do padrão nacional (Emissor Nacional, www.nfse.gov.br): o XML baixado do portal
// traz <NFSe><infNFSe Id="NFS..."> com o número, e a DPS com emissão, tomador, serviço e
// valores. O leitor não resolve entidades externas (encoding/xml não faz isso) e só
// guarda os campos de que o painel precisa.

// ErrFormatoNFSe: o arquivo não parece uma NFS-e nacional.
var ErrFormatoNFSe = errors.New("arquivo não parece o XML de uma NFS-e do Emissor Nacional")

// NFSe lida do XML.
type NFSe struct {
	Chave        string        `json:"chave"` // Id do infNFSe, sem o prefixo "NFS"
	Numero       string        `json:"numero"`
	Emissao      time.Time     `json:"emissao"`
	Competencia  time.Time     `json:"competencia"`
	PrestadorDoc string        `json:"-"`
	TomadorNome  string        `json:"tomador"`
	TomadorDoc   string        `json:"-"` // CPF/CNPJ (cifrar antes de gravar)
	TomadorPais  string        `json:"pais"`
	Descricao    string        `json:"descricao"`
	Valor        core.Centavos `json:"valor_centavos"`
	Exportacao   bool          `json:"exportacao"`
}

// LerNFSe lê um XML (um arquivo por nota, como o portal entrega).
func LerNFSe(conteudo []byte) (NFSe, error) {
	var n NFSe
	dec := xml.NewDecoder(bytes.NewReader(conteudo))
	dec.Strict = true
	var pilha []string
	dentro := func(nome string) bool {
		for _, p := range pilha {
			if p == nome {
				return true
			}
		}
		return false
	}
	var raiz string
	var valorServ, valorLiq string
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return n, ErrFormatoNFSe
		}
		switch t := tok.(type) {
		case xml.StartElement:
			nome := t.Name.Local
			if raiz == "" {
				raiz = nome
			}
			if nome == "infNFSe" {
				for _, a := range t.Attr {
					if a.Name.Local == "Id" {
						n.Chave = strings.TrimPrefix(strings.TrimSpace(a.Value), "NFS")
					}
				}
			}
			if nome == "comExt" {
				n.Exportacao = true
			}
			pilha = append(pilha, nome)
		case xml.EndElement:
			if len(pilha) > 0 {
				pilha = pilha[:len(pilha)-1]
			}
		case xml.CharData:
			if len(pilha) == 0 {
				continue
			}
			v := strings.TrimSpace(string(t))
			if v == "" {
				continue
			}
			switch atual := pilha[len(pilha)-1]; {
			case atual == "nNFSe":
				n.Numero = v
			case atual == "dhEmi" && n.Emissao.IsZero():
				if d, err := time.Parse(time.RFC3339, v); err == nil {
					n.Emissao = d
				}
			case atual == "dCompet":
				if d, err := time.Parse("2006-01-02", v); err == nil {
					n.Competencia = d
				}
			case (atual == "CNPJ" || atual == "CPF") && dentro("prest"):
				n.PrestadorDoc = v
			case (atual == "CNPJ" || atual == "CPF" || atual == "NIF") && dentro("toma"):
				n.TomadorDoc = v
			case atual == "xNome" && dentro("toma"):
				n.TomadorNome = v
			case atual == "cPais" && dentro("toma"):
				n.TomadorPais = strings.ToUpper(v)
			case atual == "xDescServ":
				n.Descricao = v
			case atual == "vServ":
				valorServ = v
			case atual == "vLiq":
				valorLiq = v
			}
		}
	}
	if raiz != "NFSe" || n.Chave == "" {
		return n, ErrFormatoNFSe
	}
	bruto := valorServ
	if bruto == "" {
		bruto = valorLiq
	}
	v, err := core.ParseDec8(bruto)
	if err != nil || v <= 0 {
		return n, ErrFormatoNFSe
	}
	n.Valor = core.Valor(v, core.Um)
	if n.Emissao.IsZero() {
		n.Emissao = n.Competencia
	}
	if n.Competencia.IsZero() {
		n.Competencia = n.Emissao
	}
	if n.Emissao.IsZero() {
		return n, ErrFormatoNFSe
	}
	if n.TomadorPais == "" {
		n.TomadorPais = "BR"
	}
	if n.TomadorPais != "BR" {
		n.Exportacao = true
	}
	return n, nil
}
