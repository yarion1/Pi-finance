// Comando pluggy-falsa: a API da Pluggy de mentira para o e2e e para testar a tela
// de Open Finance sem conta no Meu Pluggy. Não vai para a imagem de produção.
//
//	PLUGGY_FALSA=127.0.0.1:3200 go run ./cmd/pluggy-falsa
//	PLUGGY_URL=http://127.0.0.1:3200 financas serve
//
// Credenciais: Client ID "demo-client-id", Secret "demo-client-secret"; item "item-demo".
package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
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
		},
	)
	log.Printf("pluggy falsa em http://%s", endereco)
	srv := &http.Server{Addr: endereco, Handler: f, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(srv.ListenAndServe())
}
