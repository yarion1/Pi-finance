package importadores

import (
	"errors"
	"strings"
	"testing"
)

const nfseNacional = `<?xml version="1.0" encoding="utf-8"?>
<NFSe versao="1.00" xmlns="http://www.sped.fazenda.gov.br/nfse">
  <infNFSe Id="NFS35503082212345678000195000000000001226010000000123">
    <xLocEmi>São Paulo</xLocEmi>
    <nNFSe>12</nNFSe>
    <cStat>100</cStat>
    <dhProc>2026-01-10T10:00:00-03:00</dhProc>
    <DPS versao="1.00">
      <infDPS Id="DPS355030822123456780001950000100000000000012">
        <dhEmi>2026-01-10T09:30:00-03:00</dhEmi>
        <dCompet>2026-01-10</dCompet>
        <prest><CNPJ>12345678000195</CNPJ></prest>
        <toma>
          <NIF>98-7654321</NIF>
          <xNome>ACME Inc.</xNome>
          <end><endExt><cPais>US</cPais><cEndPost>94105</cEndPost></endExt></end>
        </toma>
        <serv><cServ><cTribNac>010101</cTribNac><xDescServ>Desenvolvimento de software</xDescServ></cServ>
          <comExt><mdPrestacao>1</mdPrestacao><tpMoeda>220</tpMoeda><vServMoeda>2000.00</vServMoeda></comExt>
        </serv>
        <valores><vServPrest><vServ>10880.00</vServ></vServPrest></valores>
      </infDPS>
    </DPS>
    <valores><vLiq>10880.00</vLiq></valores>
  </infNFSe>
</NFSe>`

func TestLerNFSe(t *testing.T) {
	n, err := LerNFSe([]byte(nfseNacional))
	if err != nil {
		t.Fatal(err)
	}
	if n.Chave != "35503082212345678000195000000000001226010000000123" || n.Numero != "12" || n.Valor != 1088000 ||
		n.TomadorNome != "ACME Inc." || n.TomadorDoc != "98-7654321" || n.TomadorPais != "US" || !n.Exportacao ||
		n.PrestadorDoc != "12345678000195" || n.Descricao != "Desenvolvimento de software" ||
		n.Competencia.Format("2006-01-02") != "2026-01-10" || n.Emissao.Format("2006-01-02") != "2026-01-10" {
		t.Fatalf("%+v", n)
	}

	// tomador no Brasil, sem dhEmi, só vLiq
	br := strings.NewReplacer("<NIF>98-7654321</NIF>", "<CNPJ>11222333000181</CNPJ>",
		"<end><endExt><cPais>US</cPais><cEndPost>94105</cEndPost></endExt></end>", "",
		"<comExt><mdPrestacao>1</mdPrestacao><tpMoeda>220</tpMoeda><vServMoeda>2000.00</vServMoeda></comExt>", "",
		"<dhEmi>2026-01-10T09:30:00-03:00</dhEmi>", "", "<vServ>10880.00</vServ>", "").Replace(nfseNacional)
	n, err = LerNFSe([]byte(br))
	if err != nil || n.Exportacao || n.TomadorPais != "BR" || n.Valor != 1088000 || n.Emissao.IsZero() {
		t.Fatalf("nacional: %+v %v", n, err)
	}

	for nome, xmlRuim := range map[string]string{
		"não é XML":        "isso não é xml",
		"outra raiz":       `<nfeProc><infNFSe Id="NFS1"><vServ>1.00</vServ><dCompet>2026-01-01</dCompet></infNFSe></nfeProc>`,
		"sem Id":           `<NFSe><infNFSe><vServ>1.00</vServ><dCompet>2026-01-01</dCompet></infNFSe></NFSe>`,
		"sem valor":        `<NFSe><infNFSe Id="NFS1"><dCompet>2026-01-01</dCompet></infNFSe></NFSe>`,
		"sem data":         `<NFSe><infNFSe Id="NFS1"><vServ>1.00</vServ></infNFSe></NFSe>`,
		"entidade externa": `<!DOCTYPE x [<!ENTITY e SYSTEM "file:///etc/passwd">]><NFSe><infNFSe Id="NFS1"><xDescServ>&e;</xDescServ><vServ>1</vServ><dCompet>2026-01-01</dCompet></infNFSe></NFSe>`,
	} {
		if _, err := LerNFSe([]byte(xmlRuim)); !errors.Is(err, ErrFormatoNFSe) {
			t.Errorf("%s: %v", nome, err)
		}
	}
}
