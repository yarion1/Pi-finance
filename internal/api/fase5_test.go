package api_test

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/yarion1/pi-finance/internal/financeiro"
)

type painelCNPJ struct {
	ReceitaAno int64    `json:"receita_ano_centavos"`
	Alertas    []string `json:"alertas"`
	Clientes   []struct {
		Nome       string  `json:"nome"`
		Percentual float64 `json:"percentual"`
	} `json:"clientes"`
	MEI *struct {
		Situacao string `json:"situacao"`
		DAS      int64  `json:"das_centavos"`
		Projecao struct {
			Teto  int64 `json:"teto_centavos"`
			Falta int64 `json:"falta_centavos"`
		} `json:"projecao"`
		DASMeses []struct {
			Competencia string  `json:"competencia"`
			PagoEm      *string `json:"pago_em"`
			Atrasado    bool    `json:"atrasado"`
		} `json:"das_meses"`
		Lucro struct {
			Presumido  int64 `json:"presumido_centavos"`
			Tributavel int64 `json:"tributavel_centavos"`
		} `json:"lucro"`
		Exportacao int64  `json:"exportacao_ano_centavos"`
		DASNPrazo  string `json:"dasn_prazo"`
	} `json:"mei"`
	Simples *struct {
		RBT12  int64  `json:"rbt12_centavos"`
		FatorR string `json:"fator_r"`
		Anexo  string `json:"anexo"`
		DAS    struct {
			AliquotaEfetiva string `json:"aliquota_efetiva"`
			DAS             int64  `json:"das_centavos"`
		} `json:"das"`
		ProlaboreMin int64 `json:"prolabore_para_fator_r_centavos"`
	} `json:"simples"`
}

func criarPJ(t *testing.T, a *cliente, nome, regime, abertura string) string {
	t.Helper()
	var pj struct{ ID string }
	a.exigir("POST", "/api/entidades", map[string]any{"tipo": "PJ", "nome": nome, "regime": regime,
		"atividade": "servicos", "data_abertura": abertura}, http.StatusCreated).json(t, &pj)
	return pj.ID
}

func TestMEI(t *testing.T) {
	amb, a, pf, _, _ := prepara(t)
	hoje := financeiro.Hoje()
	pj := criarPJ(t, a, "Pablo Dev MEI", "MEI", "2024-03-01")
	d := hoje.Format("2006-01-02")

	a.exigir("POST", "/api/cnpj/"+pj+"/notas", map[string]any{"data_emissao": d, "valor_centavos": 3000000,
		"cliente": "Cliente Brasil", "numero": "1"}, http.StatusCreated)
	// US$ 2.000 a 5,40: R$ 10.800, exportação
	a.exigir("POST", "/api/cnpj/"+pj+"/notas", map[string]any{"data_emissao": d, "moeda": "usd", "valor_moeda_centavos": 200000,
		"taxa_cambio": "5,40", "cliente": "ACME Inc.", "pais": "us", "exportacao": true}, http.StatusCreated)
	// sem câmbio e sem PTAX guardada: pede o câmbio
	a.exigir("POST", "/api/cnpj/"+pj+"/notas", map[string]any{"data_emissao": d, "moeda": "USD", "valor_moeda_centavos": 100}, http.StatusBadRequest)
	a.exigir("POST", "/api/cnpj/"+pj+"/notas", map[string]any{"data_emissao": d, "valor_centavos": 0}, http.StatusBadRequest)
	a.exigir("POST", "/api/cnpj/"+pf+"/notas", map[string]any{"data_emissao": d, "valor_centavos": 100}, http.StatusBadRequest)

	// NFS-e do Emissor Nacional: importar duas vezes não duplica
	xml := strings.NewReplacer("2026-01-10", d).Replace(nfseXML)
	arq := map[string]any{"arquivos": []map[string]string{{"nome": "nota.xml", "conteudo": base64.StdEncoding.EncodeToString([]byte(xml))}}}
	var imp struct {
		Novas, Duplicadas int
		Erros             []string
	}
	a.exigir("POST", "/api/cnpj/"+pj+"/notas/xml", arq, http.StatusOK).json(t, &imp)
	if imp.Novas != 1 || len(imp.Erros) != 0 {
		t.Fatalf("NFS-e: %+v", imp)
	}
	a.exigir("POST", "/api/cnpj/"+pj+"/notas/xml", arq, http.StatusOK).json(t, &imp)
	if imp.Novas != 0 || imp.Duplicadas != 1 {
		t.Fatalf("NFS-e de novo: %+v", imp)
	}
	a.exigir("POST", "/api/cnpj/"+pj+"/notas/xml", map[string]any{"arquivos": []map[string]string{{"nome": "x.xml",
		"conteudo": base64.StdEncoding.EncodeToString([]byte("<nada/>"))}}}, http.StatusOK).json(t, &imp)
	if len(imp.Erros) != 1 || !strings.Contains(imp.Erros[0], "x.xml") {
		t.Fatalf("XML inválido: %+v", imp)
	}

	// total: 30.000 + 10.800 + 20.000 (NFS-e) = 60.800 → 75 % do teto
	var p painelCNPJ
	a.exigir("GET", "/api/cnpj/"+pj+"/painel", nil, http.StatusOK).json(t, &p)
	if p.ReceitaAno != 6080000 || p.MEI == nil || p.MEI.Situacao != "atencao" || p.MEI.DAS != 8605 ||
		p.MEI.Projecao.Teto != 8100000 || p.MEI.Projecao.Falta != 2020000 || p.MEI.Exportacao != 3080000 {
		t.Fatalf("painel MEI: %+v %+v", p, p.MEI)
	}
	// lucro presumido de serviços: 32 % da receita
	if p.MEI.Lucro.Presumido != 1945600 || p.MEI.DASNPrazo != fmt.Sprintf("%d-05-31", hoje.Year()) {
		t.Fatalf("lucro/DASN: %+v", p.MEI)
	}
	atrasados := 0
	for m := 1; m <= int(hoje.Month()); m++ {
		if time.Date(hoje.Year(), time.Month(m)+1, 20, 0, 0, 0, 0, time.UTC).Before(hoje) {
			atrasados++
		}
	}
	contar := func(p painelCNPJ) (n int) {
		for _, x := range p.MEI.DASMeses {
			if x.Atrasado {
				n++
			}
		}
		return n
	}
	if contar(p) != atrasados || len(p.MEI.DASMeses) != int(hoje.Month()) {
		t.Fatalf("DAS atrasados: %d, quer %d (%+v)", contar(p), atrasados, p.MEI.DASMeses)
	}
	if atrasados > 0 {
		comp := fmt.Sprintf("%d-01", hoje.Year())
		a.exigir("PUT", "/api/cnpj/"+pj+"/das/"+comp, map[string]any{"pago_em": d}, http.StatusNoContent)
		a.exigir("GET", "/api/cnpj/"+pj+"/painel", nil, http.StatusOK).json(t, &p)
		if contar(p) != atrasados-1 || p.MEI.DASMeses[0].PagoEm == nil {
			t.Fatalf("DAS pago: %+v", p.MEI.DASMeses[0])
		}
	}
	if len(p.Clientes) != 3 || p.Clientes[0].Nome != "Cliente Brasil" {
		t.Fatalf("clientes: %+v", p.Clientes)
	}

	// cancelar uma nota tira do faturamento
	var notas []struct {
		ID, Origem string
		Numero     *string
		Moeda      string
		TaxaCambio *string `json:"taxa_cambio"`
	}
	a.exigir("GET", "/api/cnpj/"+pj+"/notas", nil, http.StatusOK).json(t, &notas)
	if len(notas) != 3 {
		t.Fatalf("notas: %+v", notas)
	}
	var usd string
	for _, n := range notas {
		if n.Moeda == "USD" {
			usd = n.ID
			if n.TaxaCambio == nil || *n.TaxaCambio != "5.4" {
				t.Fatalf("câmbio: %+v", n)
			}
		}
	}
	a.exigir("PATCH", "/api/cnpj/notas/"+usd, map[string]bool{"cancelada": true}, http.StatusNoContent)
	a.exigir("GET", "/api/cnpj/"+pj+"/painel", nil, http.StatusOK).json(t, &p)
	if p.ReceitaAno != 5000000 {
		t.Fatalf("cancelada fora: %d", p.ReceitaAno)
	}

	// pacote do contador: CSV do mês, sem fórmula vinda de texto do usuário
	a.exigir("POST", "/api/cnpj/"+pj+"/notas", map[string]any{"data_emissao": d, "valor_centavos": 100,
		"cliente": "=HYPERLINK(\"http://x\")"}, http.StatusCreated)
	csvResp := a.exigir("GET", "/api/cnpj/"+pj+"/pacote?mes="+hoje.Format("2006-01"), nil, http.StatusOK)
	corpo := string(csvResp.corpo)
	if !strings.HasPrefix(csvResp.cab.Get("Content-Type"), "text/csv") || !strings.Contains(corpo, "Cliente Brasil") ||
		!strings.Contains(corpo, "Receita bruta;") || !strings.Contains(corpo, "'=HYPERLINK") || strings.Contains(corpo, ";=HYPERLINK") {
		t.Fatalf("pacote: %s", corpo)
	}

	// o calendário fiscal entra na agenda
	var ag struct {
		Itens []struct{ Descricao, Origem string }
	}
	a.exigir("GET", "/api/agenda?dias=60", nil, http.StatusOK).json(t, &ag)
	achou := false
	for _, it := range ag.Itens {
		if it.Origem == "fiscal" && strings.HasPrefix(it.Descricao, "DAS-MEI") {
			achou = true
		}
	}
	if !achou {
		t.Fatalf("agenda sem DAS-MEI: %+v", ag.Itens)
	}

	// outra pessoa não vê nem mexe no CNPJ
	var conv struct{ Link string }
	a.exigir("POST", "/api/convites-conta", nil, http.StatusCreated).json(t, &conv)
	b := amb.novoCliente()
	b.cadastrarCom2FA("Bruno", "bruno@teste.com", conv.Link[strings.LastIndex(conv.Link, "/")+1:])
	b.exigir("GET", "/api/cnpj/"+pj+"/painel", nil, http.StatusNotFound)
	b.exigir("POST", "/api/cnpj/"+pj+"/notas", map[string]any{"data_emissao": d, "valor_centavos": 100}, http.StatusNotFound)
	b.exigir("PATCH", "/api/cnpj/notas/"+usd, map[string]bool{"cancelada": false}, http.StatusNotFound)
	b.exigir("DELETE", "/api/cnpj/notas/"+usd, nil, http.StatusNotFound)
	b.exigir("PUT", "/api/cnpj/"+pj+"/das/2026-01", map[string]any{}, http.StatusNotFound)
	a.exigir("DELETE", "/api/cnpj/notas/"+usd, nil, http.StatusNoContent)
}

// Exemplo da SPEC §4 pela API: RBT12 de R$ 240 mil e R$ 20 mil no mês. Sem folha, Anexo
// V (16,13 %, R$ 3.225); com pró-labore de R$ 5.600, Fator R de 28 % e Anexo III (7,30 %,
// R$ 1.460), INSS de R$ 616.
func TestSimplesExemploDaSPEC(t *testing.T) {
	_, a, _, _, _ := prepara(t)
	hoje := financeiro.Hoje()
	mes := time.Date(hoje.Year(), hoje.Month(), 1, 0, 0, 0, 0, time.UTC)
	pj := criarPJ(t, a, "Pablo Dev ME", "SIMPLES_ME", mes.AddDate(-2, 0, 0).Format("2006-01-02"))
	for i := 0; i <= 12; i++ {
		a.exigir("POST", "/api/cnpj/"+pj+"/notas", map[string]any{"data_emissao": mes.AddDate(0, -i, 0).Format("2006-01-02"),
			"valor_centavos": 2000000}, http.StatusCreated)
	}
	var p painelCNPJ
	a.exigir("GET", "/api/cnpj/"+pj+"/painel", nil, http.StatusOK).json(t, &p)
	if p.Simples == nil || p.Simples.RBT12 != 24000000 || p.Simples.Anexo != "V" ||
		p.Simples.DAS.AliquotaEfetiva != "0.16125000" || p.Simples.DAS.DAS != 322500 || p.Simples.ProlaboreMin != 560000 {
		t.Fatalf("Anexo V: %+v", p.Simples)
	}
	var f struct {
		INSS int64 `json:"inss_centavos"`
		IRRF int64 `json:"irrf_centavos"`
	}
	for i := 12; i >= 1; i-- { // o último é o mês passado (regras de 2026)
		a.exigir("PUT", "/api/cnpj/"+pj+"/folha", map[string]any{"competencia": mes.AddDate(0, -i, 0).Format("2006-01"),
			"prolabore_centavos": 560000}, http.StatusOK).json(t, &f)
	}
	if f.INSS != 61600 || f.IRRF != 22886 {
		t.Fatalf("INSS e IRRF do pró-labore: %+v", f)
	}
	a.exigir("PUT", "/api/cnpj/"+pj+"/folha", map[string]any{"competencia": "2026-13"}, http.StatusBadRequest)
	a.exigir("GET", "/api/cnpj/"+pj+"/painel", nil, http.StatusOK).json(t, &p)
	if p.Simples.Anexo != "III" || p.Simples.FatorR != "0.28000000" || p.Simples.DAS.AliquotaEfetiva != "0.07300000" ||
		p.Simples.DAS.DAS != 146000 {
		t.Fatalf("Anexo III: %+v", p.Simples)
	}
	var folha []struct{ Competencia string }
	a.exigir("GET", "/api/cnpj/"+pj+"/folha", nil, http.StatusOK).json(t, &folha)
	if len(folha) != 12 {
		t.Fatalf("folha: %d", len(folha))
	}

	// lucros: até R$ 50 mil no mês sem IRRF; passando, 10 % sobre o total do mês
	var av struct {
		IRRF  int64 `json:"irrf_centavos"`
		Folga int64 `json:"folga_centavos"`
	}
	d := hoje.Format("2006-01-02")
	a.exigir("POST", "/api/cnpj/"+pj+"/distribuicoes", map[string]any{"data": d, "valor_centavos": 3000000, "simular": true}, http.StatusOK).json(t, &av)
	if av.IRRF != 0 || av.Folga != 2000000 {
		t.Fatalf("simular 30 mil: %+v", av)
	}
	a.exigir("POST", "/api/cnpj/"+pj+"/distribuicoes", map[string]any{"data": d, "valor_centavos": 3000000}, http.StatusCreated)
	a.exigir("POST", "/api/cnpj/"+pj+"/distribuicoes", map[string]any{"data": d, "valor_centavos": 3000000, "simular": true}, http.StatusOK).json(t, &av)
	if av.IRRF != 600000 || av.Folga != 0 {
		t.Fatalf("passando do limite: %+v", av)
	}
	a.exigir("POST", "/api/cnpj/"+pj+"/distribuicoes", map[string]any{"data": d, "valor_centavos": 3000000}, http.StatusCreated)
	var dist []struct {
		IRRF int64 `json:"irrf_centavos"`
	}
	a.exigir("GET", "/api/cnpj/"+pj+"/distribuicoes", nil, http.StatusOK).json(t, &dist)
	if len(dist) != 2 || dist[0].IRRF+dist[1].IRRF != 600000 {
		t.Fatalf("distribuições: %+v", dist)
	}
}

func TestSimuladorMigracao(t *testing.T) {
	_, a, _, _, _ := prepara(t)
	var s struct {
		Cenarios []struct {
			Nome      string
			Possivel  bool
			Anexo     string
			Impostos  int64 `json:"impostos_centavos"`
			Prolabore int64 `json:"prolabore_mensal_centavos"`
			INSS      int64 `json:"inss_anual_centavos"`
			IRRF      int64 `json:"irrf_anual_centavos"`
			Total     int64 `json:"total_anual_centavos"`
		}
		Melhor string
	}
	// R$ 10 mil por mês (R$ 120 mil no ano) e contador de R$ 300: o MEI não cabe
	a.exigir("GET", "/api/cnpj/simulacao?receita_mensal_centavos=1000000&contador_centavos=30000", nil, http.StatusOK).json(t, &s)
	mei, v, iii := s.Cenarios[0], s.Cenarios[1], s.Cenarios[2]
	if mei.Possivel || mei.Impostos != 8605*12 {
		t.Fatalf("MEI: %+v", mei)
	}
	// Anexo V: 15,5 % → R$ 18.600; INSS de 1 salário mínimo (178,31 × 12); contador 3.600
	if v.Impostos != 1860000 || v.INSS != 213972 || v.Total != 1860000+213972+360000 {
		t.Fatalf("Anexo V: %+v", v)
	}
	// Anexo III: 6 % → R$ 7.200; pró-labore de R$ 2.800 (INSS 308 × 12), sem IRRF
	if iii.Impostos != 720000 || iii.Prolabore != 280000 || iii.INSS != 369600 || iii.IRRF != 0 ||
		iii.Total != 720000+369600+360000 || s.Melhor != iii.Nome {
		t.Fatalf("Anexo III: %+v (melhor %s)", iii, s.Melhor)
	}
	// R$ 5 mil por mês: MEI é o melhor
	a.exigir("GET", "/api/cnpj/simulacao?receita_mensal_centavos=500000&contador_centavos=30000", nil, http.StatusOK).json(t, &s)
	if !s.Cenarios[0].Possivel || s.Melhor != s.Cenarios[0].Nome {
		t.Fatalf("MEI cabe: %+v", s)
	}
	a.exigir("GET", "/api/cnpj/simulacao?receita_mensal_centavos=x", nil, http.StatusBadRequest)
}

const nfseXML = `<?xml version="1.0" encoding="utf-8"?>
<NFSe versao="1.00" xmlns="http://www.sped.fazenda.gov.br/nfse">
  <infNFSe Id="NFS35503082212345678000195000000000001226010000000123">
    <nNFSe>12</nNFSe>
    <DPS versao="1.00"><infDPS Id="DPS1">
      <dhEmi>2026-01-10T09:30:00-03:00</dhEmi>
      <dCompet>2026-01-10</dCompet>
      <prest><CNPJ>12345678000195</CNPJ></prest>
      <toma><NIF>98-7654321</NIF><xNome>ACME Global</xNome><end><endExt><cPais>US</cPais></endExt></end></toma>
      <serv><cServ><xDescServ>Desenvolvimento de software</xDescServ></cServ></serv>
      <valores><vServPrest><vServ>20000.00</vServ></vServPrest></valores>
    </infDPS></DPS>
  </infNFSe>
</NFSe>`
