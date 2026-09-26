// Comando pluggy-falsa: as APIs de mentira do e2e — a da Pluggy (para testar o Open
// Finance sem conta no Meu Pluggy) e a do Claude em /v1/messages (para testar a IA sem
// chave nem custo). Não vai para a imagem de produção.
//
//	PLUGGY_FALSA=127.0.0.1:3200 go run ./cmd/pluggy-falsa
//	PLUGGY_URL=http://127.0.0.1:3200 ANTHROPIC_API_KEY=teste ANTHROPIC_BASE_URL=http://127.0.0.1:3200 financas serve
//
// Credenciais: Client ID "demo-client-id", Secret "demo-client-secret"; item "item-demo".
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/yarion1/pi-finance/internal/pluggy"
	"github.com/yarion1/pi-finance/internal/pluggy/pluggyfalsa"
)

func main() {
	endereco := os.Getenv("PLUGGY_FALSA")
	if endereco == "" {
		endereco = "127.0.0.1:3200"
	}
	sp, err := time.LoadLocation("America/Sao_Paulo")
	if err != nil {
		sp = time.FixedZone("BRT", -3*3600)
	}
	hoje := time.Now().In(sp)
	dia := func(n int) string { return hoje.AddDate(0, 0, n).Format("2006-01-02") + "T15:00:00.000Z" }
	tx := func(id string, n int, valor, tipo, descricao string) pluggy.Transacao {
		return pluggy.Transacao{ID: id, Date: dia(n), Amount: json.Number(valor), Type: tipo, Status: "POSTED", Description: descricao}
	}

	f := pluggyfalsa.Nova()
	f.PorPagina = 50
	f.Cliente("demo-client-id", "demo-client-secret")
	f.Item("demo-client-id", "item-demo", "Nubank",
		&pluggyfalsa.Conta{
			Conta: pluggy.Conta{ID: "conta-demo", Type: "BANK", Subtype: "CHECKING_ACCOUNT", Name: "Conta Nubank", Balance: "2345.67", CurrencyCode: "BRL"},
			Transacoes: []pluggy.Transacao{
				tx("d1", -6, "5000", "CREDIT", "SALARIO EMPRESA LTDA"),
				tx("d2", -4, "-89.90", "DEBIT", "SUPERMERCADO DIA"),
				tx("d3", -2, "-32.50", "DEBIT", "UBER *TRIP"),
			},
		},
		&pluggyfalsa.Conta{
			Conta:      pluggy.Conta{ID: "cartao-demo", Type: "CREDIT", Subtype: "CREDIT_CARD", Name: "Nubank Ultravioleta", Balance: "450.00", CurrencyCode: "BRL"},
			Transacoes: []pluggy.Transacao{tx("c1", -3, "450", "DEBIT", "RESTAURANTE")},
			Faturas: []pluggy.Fatura{{ID: "fat-1", DueDate: dia(-15), TotalAmount: "1234.56", MinimumPayment: "185.18",
				CurrencyCode: "BRL", FinanceCharges: []pluggy.EncargoFatura{{Type: "IOF", Amount: "3.45"}}}},
		},
	)
	f.Investimentos("item-demo",
		pluggy.Investimento{ID: "inv-1", Name: "CDB - PICPAY INSTITUICAO DE PAGAMENTO S/A", Type: "FIXED_INCOME", Subtype: "CDB",
			Balance: "12726.64", AmountOriginal: "12000", Issuer: "PicPay", Status: "ACTIVE"},
		pluggy.Investimento{ID: "inv-2", Name: "LCI BANCO X", Type: "FIXED_INCOME", Subtype: "LCI", Balance: "5000",
			AmountOriginal: "4800", Issuer: "Banco X", Status: "ACTIVE"},
		pluggy.Investimento{ID: "inv-3", Name: "CDB - PICPAY INSTITUICAO DE PAGAMENTO S/A", Type: "FIXED_INCOME", Subtype: "CDB",
			Balance: "0", Status: "TOTAL_WITHDRAWAL"},
	)
	pagas, total := 5, 12
	f.Extras("item-demo",
		[]pluggy.Emprestimo{{ID: "emp-1", ProductName: "Crédito pessoal", Kind: "LOAN", ContractAmount: "10000",
			CurrencyCode: "BRL", CET: "0.035", Installments: &pluggy.ParcelasEmprestimo{TotalNumberOfInstallments: &total, PaidInstallments: &pagas},
			Payments: &pluggy.PagamentosEmprestimo{ContractOutstandingBalance: "6543.21"}}},
		nil,
		map[string][]pluggy.MovimentoInvestimento{"inv-1": {{ID: "mov-1", MovementType: "CREDIT", Amount: "12000", Date: dia(-400)}}})
	log.Printf("pluggy falsa em http://%s", endereco)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/messages", claudeFalso)
	mux.Handle("/", f)
	srv := &http.Server{Addr: endereco, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

// claudeFalso: categoriza nada e, no chat, pede gastos_por_categoria do mês e depois
// responde com o que a ferramenta devolveu.
func claudeFalso(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
		Messages []struct {
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	corpo, _ := io.ReadAll(io.LimitReader(r.Body, 8<<20))
	_ = json.Unmarshal(corpo, &p)
	resp := func(stop string, bloco string) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"id":"msg","type":"message","role":"assistant","model":"falso","stop_reason":%q,"content":[%s],"usage":{"input_tokens":500,"output_tokens":50}}`, stop, bloco)
	}
	if len(p.Tools) > 0 && p.Tools[0].Name == "classificar" {
		resp("tool_use", `{"type":"tool_use","id":"tu","name":"classificar","input":{"itens":[]}}`)
		return
	}
	ultimo := ""
	if n := len(p.Messages); n > 0 {
		ultimo = string(p.Messages[n-1].Content)
	}
	if strings.Contains(ultimo, "tool_result") {
		texto, _ := json.Marshal("Resposta de teste com os números da ferramenta: " + resumir(ultimo))
		resp("end_turn", `{"type":"text","text":`+string(texto)+`}`)
		return
	}
	mes := time.Now().Format("2006-01")
	resp("tool_use", `{"type":"tool_use","id":"tu_1","name":"gastos_por_categoria","input":{"mes":"`+mes+`"}}`)
}

func resumir(s string) string {
	if len(s) > 300 {
		return s[:300]
	}
	return s
}
