package notificar

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Telegram: o bot do painel (token do BotFather em TELEGRAM_BOT_TOKEN). O worker lê as
// mensagens novas de tempos em tempos (getUpdates), sem webhook: funciona atrás do
// túnel e não abre porta.
type Telegram struct {
	Token string
	Base  string // https://api.telegram.org (os testes trocam)
	HTTP  *http.Client
}

// NovoTelegram ou nil sem token.
func NovoTelegram(token string) *Telegram {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil
	}
	return &Telegram{Token: token, Base: "https://api.telegram.org", HTTP: &http.Client{Timeout: 20 * time.Second}}
}

// chamar um método da API. O token vai no caminho da URL: os erros nunca levam a URL.
func (t *Telegram) chamar(ctx context.Context, metodo string, corpo any, destino any) error {
	j, err := json.Marshal(corpo)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.Base+"/bot"+t.Token+"/"+metodo, bytes.NewReader(j))
	if err != nil {
		return fmt.Errorf("telegram: %s: pedido inválido", metodo)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.HTTP.Do(req)
	if err != nil {
		var ue *url.Error
		if errors.As(err, &ue) {
			err = ue.Err // sem a URL (que tem o token)
		}
		return fmt.Errorf("telegram: %s: %w", metodo, err)
	}
	defer resp.Body.Close()
	var r struct {
		OK          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r); err != nil || !r.OK {
		return fmt.Errorf("telegram: %s: %d %s", metodo, resp.StatusCode, r.Description)
	}
	if destino != nil {
		return json.Unmarshal(r.Result, destino)
	}
	return nil
}

// Usuario do bot (para o link t.me/<usuario>).
func (t *Telegram) Usuario(ctx context.Context) (string, error) {
	var eu struct {
		Username string `json:"username"`
	}
	err := t.chamar(ctx, "getMe", map[string]any{}, &eu)
	return eu.Username, err
}

// Mensagem recebida pelo bot.
type Mensagem struct {
	ID     int64
	ChatID int64
	Texto  string
}

// Novas mensagens desde o deslocamento (o próximo deslocamento é o maior ID + 1).
func (t *Telegram) Novas(ctx context.Context, desde int64) ([]Mensagem, error) {
	var ups []struct {
		UpdateID int64 `json:"update_id"`
		Message  *struct {
			Chat struct {
				ID   int64  `json:"id"`
				Type string `json:"type"`
			} `json:"chat"`
			Text string `json:"text"`
		} `json:"message"`
	}
	if err := t.chamar(ctx, "getUpdates", map[string]any{"offset": desde, "timeout": 0, "allowed_updates": []string{"message"}}, &ups); err != nil {
		return nil, err
	}
	out := []Mensagem{}
	for _, u := range ups {
		m := Mensagem{ID: u.UpdateID}
		if u.Message != nil && u.Message.Chat.Type == "private" {
			m.ChatID, m.Texto = u.Message.Chat.ID, u.Message.Text
		}
		out = append(out, m)
	}
	return out, nil
}

// Enviar texto simples para o chat.
func (t *Telegram) Enviar(ctx context.Context, chatID int64, texto string) error {
	return t.chamar(ctx, "sendMessage", map[string]any{"chat_id": chatID, "text": texto,
		"link_preview_options": map[string]bool{"is_disabled": true}}, nil)
}

// CodigoDoStart: "/start CODIGO" → CODIGO (o link t.me/bot?start=CODIGO manda isso).
func CodigoDoStart(texto string) string {
	c, ok := strings.CutPrefix(strings.TrimSpace(texto), "/start ")
	if !ok {
		return ""
	}
	return strings.TrimSpace(c)
}

// ChatTexto: o chat_id guardado (cifrado) como texto.
func ChatTexto(id int64) string { return strconv.FormatInt(id, 10) }
