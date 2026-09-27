package notificar

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTelegram(t *testing.T) {
	if NovoTelegram("  ") != nil {
		t.Fatal("sem token, sem bot")
	}
	var enviados []map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corpo, _ := io.ReadAll(r.Body)
		var p map[string]any
		_ = json.Unmarshal(corpo, &p)
		switch r.URL.Path {
		case "/botTOKEN-SECRETO/getMe":
			_, _ = io.WriteString(w, `{"ok":true,"result":{"username":"financas_bot"}}`)
		case "/botTOKEN-SECRETO/getUpdates":
			if p["offset"] != float64(7) {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_, _ = io.WriteString(w, `{"ok":true,"result":[
				{"update_id":7,"message":{"chat":{"id":123,"type":"private"},"text":"/start ABC_def-1"}},
				{"update_id":8,"message":{"chat":{"id":-99,"type":"group"},"text":"/start X"}},
				{"update_id":9}]}`)
		case "/botTOKEN-SECRETO/sendMessage":
			enviados = append(enviados, p)
			_, _ = io.WriteString(w, `{"ok":true,"result":{}}`)
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"ok":false,"description":"Unauthorized"}`)
		}
	}))
	defer srv.Close()
	tg := NovoTelegram("TOKEN-SECRETO")
	tg.Base = srv.URL
	ctx := context.Background()
	if u, err := tg.Usuario(ctx); err != nil || u != "financas_bot" {
		t.Fatalf("getMe: %q %v", u, err)
	}
	ms, err := tg.Novas(ctx, 7)
	if err != nil || len(ms) != 3 || ms[0].ChatID != 123 || CodigoDoStart(ms[0].Texto) != "ABC_def-1" || ms[1].ChatID != 0 || ms[2].ID != 9 {
		t.Fatalf("mensagens: %+v %v", ms, err)
	}
	if err := tg.Enviar(ctx, 123, "oi"); err != nil || len(enviados) != 1 || enviados[0]["text"] != "oi" {
		t.Fatalf("envio: %v %+v", err, enviados)
	}
	// erros nunca mostram o token
	ruim := NovoTelegram("OUTRO-TOKEN")
	ruim.Base = srv.URL
	if _, err := ruim.Usuario(ctx); err == nil || strings.Contains(err.Error(), "OUTRO-TOKEN") {
		t.Fatalf("erro da API: %v", err)
	}
	fora := NovoTelegram("TOKEN-QUE-NAO-APARECE")
	fora.Base = "http://127.0.0.1:1"
	if err := fora.Enviar(ctx, 1, "x"); err == nil || strings.Contains(err.Error(), "TOKEN-QUE-NAO-APARECE") {
		t.Fatalf("erro de rede: %v", err)
	}
	if _, err := tg.Novas(ctx, 1); err == nil {
		t.Fatal("offset errado")
	}
	if CodigoDoStart("oi") != "" || ChatTexto(-5) != "-5" {
		t.Fatal("auxiliares")
	}
}
