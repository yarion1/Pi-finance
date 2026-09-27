package api_test

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/yarion1/pi-finance/internal/cripto"
	"github.com/yarion1/pi-finance/internal/financeiro"
	"github.com/yarion1/pi-finance/internal/notificar"
	"github.com/yarion1/pi-finance/internal/worker"
)

// navegador: chaves de um aparelho inscrito (o servidor de push falso decifra com elas).
type navegador struct {
	priv *ecdh.PrivateKey
	auth []byte
}

func novoNavegador(t *testing.T) navegador {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	a := make([]byte, 16)
	_, _ = rand.Read(a)
	return navegador{priv: priv, auth: a}
}

func (n navegador) inscricao(endpoint string) map[string]string {
	b := base64.RawURLEncoding
	return map[string]string{"endpoint": endpoint, "p256dh": b.EncodeToString(n.priv.PublicKey().Bytes()),
		"auth": b.EncodeToString(n.auth), "aparelho": "Android · Chrome"}
}

// decifrar como o navegador (RFC 8291).
func (n navegador) decifrar(t *testing.T, corpo []byte) notificar.Aviso {
	t.Helper()
	sal, tam := corpo[:16], int(corpo[20])
	asPub := corpo[21 : 21+tam]
	chave, err := ecdh.P256().NewPublicKey(asPub)
	if err != nil {
		t.Fatal(err)
	}
	segredo, _ := n.priv.ECDH(chave)
	info := append(append([]byte("WebPush: info\x00"), n.priv.PublicKey().Bytes()...), asPub...)
	ikm, _ := hkdf.Key(sha256.New, segredo, n.auth, string(info), 32)
	cek, _ := hkdf.Key(sha256.New, ikm, sal, "Content-Encoding: aes128gcm\x00", 16)
	nonce, _ := hkdf.Key(sha256.New, ikm, sal, "Content-Encoding: nonce\x00", 12)
	bloco, _ := aes.NewCipher(cek)
	gcm, _ := cipher.NewGCM(bloco)
	claro, err := gcm.Open(nil, nonce, corpo[21+tam:], nil)
	if err != nil {
		t.Fatalf("decifrar: %v", err)
	}
	var a notificar.Aviso
	if err := json.Unmarshal(claro[:len(claro)-1], &a); err != nil {
		t.Fatal(err)
	}
	return a
}

// servicos falsos: push (qualquer host cai aqui) e API do Telegram.
type servicosFalsos struct {
	mu         sync.Mutex
	push       [][]byte
	statusPush int
	updates    []string // JSON dos updates que o getUpdates devolve (uma vez)
	offsets    []float64
	telegram   []map[string]any
}

func (f *servicosFalsos) enviadosPush() [][]byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	p := f.push
	f.push = nil
	return p
}

func (f *servicosFalsos) mensagensTelegram() []map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := f.telegram
	f.telegram = nil
	return m
}

func ligarNotificacoes(t *testing.T, amb *ambiente) (*notificar.Despachante, *servicosFalsos) {
	t.Helper()
	f := &servicosFalsos{statusPush: http.StatusCreated}
	push := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		corpo, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		defer f.mu.Unlock()
		if !strings.HasPrefix(r.Header.Get("Authorization"), "vapid t=") {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		f.push = append(f.push, corpo)
		w.WriteHeader(f.statusPush)
	}))
	t.Cleanup(push.Close)
	tr := push.Client().Transport.(*http.Transport).Clone()
	tr.DialContext = func(ctx context.Context, rede, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, rede, push.Listener.Addr().String())
	}
	tr.TLSClientConfig.ServerName = "example.com"

	tg := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p map[string]any
		_ = json.NewDecoder(r.Body).Decode(&p)
		f.mu.Lock()
		defer f.mu.Unlock()
		switch r.URL.Path {
		case "/botTOKEN/getMe":
			_, _ = io.WriteString(w, `{"ok":true,"result":{"username":"financas_teste_bot"}}`)
		case "/botTOKEN/getUpdates":
			f.offsets = append(f.offsets, p["offset"].(float64))
			_, _ = io.WriteString(w, `{"ok":true,"result":[`+strings.Join(f.updates, ",")+`]}`)
			f.updates = nil
		case "/botTOKEN/sendMessage":
			f.telegram = append(f.telegram, p)
			_, _ = io.WriteString(w, `{"ok":true,"result":{}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(tg.Close)

	cif, _ := cripto.Novo(make([]byte, 32))
	vapid, err := notificar.VAPIDDaSemente(cif.Derivar("financas vapid"), "mailto:teste@exemplo.com")
	if err != nil {
		t.Fatal(err)
	}
	telegram := notificar.NovoTelegram("TOKEN")
	telegram.Base = tg.URL
	d := &notificar.Despachante{Pool: amb.banco.App, Cifrador: amb.srv.Cifrador, URLPublica: origemTeste, Telegram: telegram,
		Push: &notificar.Push{VAPID: vapid, HTTP: &http.Client{Transport: tr}}}
	amb.srv.Notificar = d
	return d, f
}

func TestNotificacoes(t *testing.T) {
	amb, a, pf, nu, _ := prepara(t)
	ctx := context.Background()
	var casa struct{ ID string }
	a.exigir("POST", "/api/casas", map[string]string{"nome": "Casa"}, http.StatusCreated).json(t, &casa)
	var convite struct{ Link string }
	a.exigir("POST", "/api/casas/"+casa.ID+"/convites", map[string]string{"papel": "membro"}, http.StatusCreated).json(t, &convite)
	b := amb.novoCliente()
	b.cadastrarCom2FA("Bia", "bia@teste.com", convite.Link[strings.LastIndex(convite.Link, "/")+1:])

	// sem despachante: tudo indisponível, e a tela ainda abre
	var e struct {
		PushDisponivel     bool   `json:"push_disponivel"`
		ChavePublica       string `json:"chave_publica"`
		TelegramDisponivel bool   `json:"telegram_disponivel"`
		TelegramConectado  bool   `json:"telegram_conectado"`
		Aparelhos          []struct{ ID, Aparelho string }
		Preferencias       notificar.Preferencias
		Tipos              []string
	}
	a.exigir("GET", "/api/notificacoes", nil, http.StatusOK).json(t, &e)
	if e.PushDisponivel || e.TelegramDisponivel || len(e.Tipos) != 4 {
		t.Fatalf("sem despachante: %+v", e)
	}
	a.exigir("POST", "/api/notificacoes/teste", nil, http.StatusServiceUnavailable)

	d, f := ligarNotificacoes(t, amb)
	a.exigir("GET", "/api/notificacoes", nil, http.StatusOK).json(t, &e)
	if !e.PushDisponivel || e.ChavePublica != d.Push.VAPID.Publica || !e.TelegramDisponivel || e.TelegramConectado || len(e.Aparelhos) != 0 {
		t.Fatalf("estado: %+v", e)
	}

	// inscrição: só serviços de push conhecidos (SSRF) e chaves válidas
	nav := novoNavegador(t)
	endpoint := "https://fcm.googleapis.com/fcm/send/aparelho-da-ana"
	for _, ruim := range []string{"https://127.0.0.1/x", "http://fcm.googleapis.com/x", "https://intranet.local/fcm.googleapis.com"} {
		a.exigir("POST", "/api/notificacoes/push", nav.inscricao(ruim), http.StatusBadRequest)
	}
	quebrada := nav.inscricao(endpoint)
	quebrada["p256dh"] = "AAAA"
	a.exigir("POST", "/api/notificacoes/push", quebrada, http.StatusBadRequest)
	a.exigir("POST", "/api/notificacoes/push", nav.inscricao(endpoint), http.StatusNoContent)
	a.exigir("POST", "/api/notificacoes/push", nav.inscricao(endpoint), http.StatusNoContent) // reinscrição não duplica
	// outra pessoa não toma o aparelho de A
	b.exigir("POST", "/api/notificacoes/push", novoNavegador(t).inscricao(endpoint), http.StatusNoContent)
	a.exigir("GET", "/api/notificacoes", nil, http.StatusOK).json(t, &e)
	if len(e.Aparelhos) != 1 || e.Aparelhos[0].Aparelho != "Android · Chrome" {
		t.Fatalf("aparelhos: %+v", e.Aparelhos)
	}
	idAparelho := e.Aparelhos[0].ID
	var eb struct{ Aparelhos []struct{ ID string } }
	b.exigir("GET", "/api/notificacoes", nil, http.StatusOK).json(t, &eb)
	if len(eb.Aparelhos) != 0 {
		t.Fatalf("B vê aparelho: %+v", eb)
	}
	b.exigir("DELETE", "/api/notificacoes/push/"+idAparelho, nil, http.StatusNotFound)

	// o endereço e as chaves ficam cifrados no banco
	var guardada string
	if err := amb.banco.Dono.QueryRow(ctx, "select inscricao_cifrada from push_inscricoes").Scan(&guardada); err != nil ||
		strings.Contains(guardada, "fcm.googleapis.com") {
		t.Fatalf("inscrição em claro: %q %v", guardada, err)
	}

	// Telegram: o link pede a senha de novo; o código vale uma vez
	a.exigir("POST", "/api/notificacoes/telegram", nil, http.StatusForbidden)
	a.exigir("POST", "/api/auth/reautenticar", map[string]string{"senha": "senha comprida de teste"}, http.StatusNoContent)
	var lk struct{ Link string }
	a.exigir("POST", "/api/notificacoes/telegram", nil, http.StatusOK).json(t, &lk)
	codigo, ok := strings.CutPrefix(lk.Link, "https://t.me/financas_teste_bot?start=")
	if !ok || len(codigo) < 30 {
		t.Fatalf("link: %s", lk.Link)
	}
	f.updates = []string{
		fmt.Sprintf(`{"update_id":40,"message":{"chat":{"id":555,"type":"private"},"text":"/start %s"}}`, codigo),
		fmt.Sprintf(`{"update_id":41,"message":{"chat":{"id":666,"type":"private"},"text":"/start %s"}}`, codigo), // reuso
		`{"update_id":42,"message":{"chat":{"id":777,"type":"private"},"text":"oi"}}`,
	}
	if err := d.VincularTelegram(ctx); err != nil {
		t.Fatal(err)
	}
	msgs := f.mensagensTelegram()
	if len(msgs) != 3 || msgs[0]["chat_id"] != float64(555) || !strings.HasPrefix(msgs[0]["text"].(string), "Pronto") ||
		!strings.Contains(msgs[1]["text"].(string), "venceu") || !strings.Contains(msgs[2]["text"].(string), "Notificações") {
		t.Fatalf("respostas do bot: %+v", msgs)
	}
	if err := d.VincularTelegram(ctx); err != nil || f.offsets[len(f.offsets)-1] != 43 {
		t.Fatalf("deslocamento: %v %v", f.offsets, err)
	}
	a.exigir("GET", "/api/notificacoes", nil, http.StatusOK).json(t, &e)
	b.exigir("GET", "/api/notificacoes", nil, http.StatusOK).json(t, &eb)
	if !e.TelegramConectado {
		t.Fatal("Telegram deveria estar ligado")
	}

	// preferências: alertas só no aparelho; o resto nos dois
	a.exigir("PUT", "/api/notificacoes", map[string]any{"tipos": map[string]any{
		"alertas": map[string]bool{"push": true, "telegram": false}, "desconhecido": map[string]bool{"push": true}},
		"mostrar_valores": false}, http.StatusNoContent)
	a.exigir("GET", "/api/notificacoes", nil, http.StatusOK).json(t, &e)
	if _, tem := e.Preferencias.Tipos["desconhecido"]; tem || e.Preferencias.Quer("alertas", true) || !e.Preferencias.Quer("agenda", true) {
		t.Fatalf("preferências: %+v", e.Preferencias)
	}

	// alerta novo: vai por push, sem valor na tela bloqueada; uma vez só
	usuarioA := usuarioDe(t, a)
	if _, err := amb.banco.Dono.Exec(ctx, `insert into alertas (usuario_id, tipo, dados) values ($1, 'cobranca_duplicada',
		'{"descricao":"NETFLIX","valor_centavos":-5590,"data":"2026-09-20","conta":"Nubank"}')`, usuarioA); err != nil {
		t.Fatal(err)
	}
	cedo := time.Date(2026, 9, 27, 6, 0, 0, 0, time.UTC) // antes das 8 h: sem agenda
	if err := worker.Notificar(ctx, d, cedo); err != nil {
		t.Fatal(err)
	}
	push := f.enviadosPush()
	if len(push) != 1 || len(f.mensagensTelegram()) != 0 {
		t.Fatalf("alerta: %d push", len(push))
	}
	av := nav.decifrar(t, push[0])
	if av.Titulo != "Possível cobrança duplicada: NETFLIX" || !strings.Contains(av.Texto, "•••") || strings.Contains(av.Texto, "55") ||
		av.Link != "/gastos?busca=NETFLIX" {
		t.Fatalf("aviso: %+v", av)
	}
	if err := worker.Notificar(ctx, d, cedo); err != nil || len(f.enviadosPush()) != 0 {
		t.Fatal("o alerta saiu duas vezes")
	}

	// com valores ligados
	a.exigir("PUT", "/api/notificacoes", map[string]any{"tipos": map[string]any{}, "mostrar_valores": true}, http.StatusNoContent)
	if _, err := amb.banco.Dono.Exec(ctx, `insert into alertas (usuario_id, tipo, dados) values ($1, 'tarifa_bancaria',
		'{"descricao":"TARIFA PACOTE","valor_centavos":-3500,"data":"2026-09-21"}')`, usuarioA); err != nil {
		t.Fatal(err)
	}
	if err := worker.Notificar(ctx, d, cedo); err != nil {
		t.Fatal(err)
	}
	push = f.enviadosPush()
	msgs = f.mensagensTelegram()
	if len(push) != 1 || len(msgs) != 1 || !strings.Contains(nav.decifrar(t, push[0]).Texto, "R$ 35,00") ||
		!strings.Contains(msgs[0]["text"].(string), origemTeste+"/gastos?busca=TARIFA+PACOTE") {
		t.Fatalf("com valores: %d push, %+v", len(push), msgs)
	}

	// agenda: contas de hoje e amanhã, uma vez por dia, depois das 8 h
	hoje := financeiro.Hoje()
	a.exigir("POST", "/api/compromissos", map[string]any{"entidade_id": pf, "descricao": "IPVA", "valor_centavos": -30000,
		"vencimento": hoje.Format("2006-01-02"), "conta_id": nu}, http.StatusCreated)
	a.exigir("POST", "/api/compromissos", map[string]any{"entidade_id": pf, "descricao": "Condomínio", "valor_centavos": -80000,
		"vencimento": hoje.AddDate(0, 0, 1).Format("2006-01-02"), "conta_id": nu}, http.StatusCreated)
	a.exigir("POST", "/api/compromissos", map[string]any{"entidade_id": pf, "descricao": "Seguro", "valor_centavos": -10000,
		"vencimento": hoje.AddDate(0, 0, 5).Format("2006-01-02"), "conta_id": nu}, http.StatusCreated)
	manha := time.Date(hoje.Year(), hoje.Month(), hoje.Day(), 9, 0, 0, 0, time.UTC)
	if err := worker.Notificar(ctx, d, manha); err != nil {
		t.Fatal(err)
	}
	push = f.enviadosPush()
	msgs = f.mensagensTelegram()
	if len(push) != 1 || len(msgs) != 1 {
		t.Fatalf("agenda: %d push, %d telegram", len(push), len(msgs))
	}
	av = nav.decifrar(t, push[0])
	if av.Titulo != "2 contas para hoje e amanhã (R$ 1.100,00)" || av.Texto != "IPVA, Condomínio" || av.Link != "/agenda" {
		t.Fatalf("agenda: %+v", av)
	}
	if err := worker.Notificar(ctx, d, manha.Add(time.Hour)); err != nil || len(f.enviadosPush()) != 0 {
		t.Fatal("a agenda saiu duas vezes no dia")
	}

	// teste manual: todos os canais
	var teste struct{ Enviados int }
	a.exigir("POST", "/api/notificacoes/teste", nil, http.StatusOK).json(t, &teste)
	if teste.Enviados != 2 || len(f.enviadosPush()) != 1 || len(f.mensagensTelegram()) != 1 {
		t.Fatalf("teste: %+v", teste)
	}
	b.exigir("POST", "/api/notificacoes/teste", nil, http.StatusOK).json(t, &teste)
	if teste.Enviados != 0 || len(f.enviadosPush()) != 0 {
		t.Fatalf("B não tem canais: %+v", teste)
	}

	// aparelho que o serviço diz não existir mais sai da lista
	f.statusPush = http.StatusGone
	a.exigir("POST", "/api/notificacoes/teste", nil, http.StatusOK).json(t, &teste)
	a.exigir("GET", "/api/notificacoes", nil, http.StatusOK).json(t, &e)
	if len(e.Aparelhos) != 0 || teste.Enviados != 1 {
		t.Fatalf("inscrição vencida: %+v %+v", e.Aparelhos, teste)
	}

	// desligar o Telegram e remover aparelho
	f.statusPush = http.StatusCreated
	a.exigir("POST", "/api/notificacoes/push", nav.inscricao(endpoint), http.StatusNoContent)
	a.exigir("GET", "/api/notificacoes", nil, http.StatusOK).json(t, &e)
	a.exigir("DELETE", "/api/notificacoes/push/"+e.Aparelhos[0].ID, nil, http.StatusNoContent)
	a.exigir("DELETE", "/api/notificacoes/telegram", nil, http.StatusNoContent)
	a.exigir("GET", "/api/notificacoes", nil, http.StatusOK).json(t, &e)
	if e.TelegramConectado || len(e.Aparelhos) != 0 {
		t.Fatalf("desligar: %+v", e)
	}
}
