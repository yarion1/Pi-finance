package notificar

import (
	"context"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"errors"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Vetor do apêndice A da RFC 8291.
func TestCifrarVetorDaRFC8291(t *testing.T) {
	d := func(s string) []byte {
		b, err := b64.DecodeString(s)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	as, err := ecdh.P256().NewPrivateKey(d("yfWPiYE-n46HLnH0KqZOF1fJJU3MYrct3AELtAQ-oRw"))
	if err != nil {
		t.Fatal(err)
	}
	if b64.EncodeToString(as.PublicKey().Bytes()) != "BP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A8" {
		t.Fatal("chave pública do servidor")
	}
	insc := Inscricao{P256dh: "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4",
		Auth: "BTBZMqHH6r4Tts7J_aSIgg"}
	corpo, err := Cifrar([]byte("When I grow up, I want to be a watermelon"), insc, d("DGv6ra1nlYgDCS1FRnbzlw"), as)
	if err != nil {
		t.Fatal(err)
	}
	quer := "DGv6ra1nlYgDCS1FRnbzlwAAEABBBP4z9KsN6nGRTbVYI_c7VJSPQTBtkgcy27mlmlMoZIIgDll6e3vCYLocInmYWAmS6TlzAC8wEqKK6PBru3jl7A_yl95bQpu6cVPTpK4Mqgkf1CXztLVBSt2Ks3oZwbuwXPXLWyouBWLVWGNWQexSgSxsj_Qulcy4a-fN"
	if b64.EncodeToString(corpo) != quer {
		t.Fatalf("corpo cifrado diferente do vetor da RFC:\n%s", b64.EncodeToString(corpo))
	}
	// chaves inválidas
	for _, i := range []Inscricao{{P256dh: "!", Auth: insc.Auth}, {P256dh: insc.P256dh, Auth: "curto"}, {P256dh: "AAAA", Auth: insc.Auth}} {
		if _, err := Cifrar([]byte("x"), i, nil, nil); err == nil {
			t.Fatalf("inscrição inválida aceita: %+v", i)
		}
	}
	// sem sal e chave efêmera: gera novos (tamanho do cabeçalho confere)
	c, err := Cifrar([]byte("oi"), insc, nil, nil)
	if err != nil || len(c) != 16+4+1+65+2+1+16 {
		t.Fatalf("aleatório: %d %v", len(c), err)
	}
}

func TestValidarEndpoint(t *testing.T) {
	bons := []string{"https://fcm.googleapis.com/fcm/send/abc", "https://updates.push.services.mozilla.com/wpush/v2/x",
		"https://web.push.apple.com/QGx", "https://wns2-par02p.notify.windows.com/w/?token=x"}
	for _, e := range bons {
		if err := ValidarEndpoint(e); err != nil {
			t.Errorf("%s: %v", e, err)
		}
	}
	ruins := []string{"http://fcm.googleapis.com/x", "https://evil.com/fcm.googleapis.com", "https://fcm.googleapis.com.evil.com/x",
		"https://127.0.0.1/x", "https://user@fcm.googleapis.com/x", "https://fcm.googleapis.com:8443/x", "::", "https://apple.com.push/x"}
	for _, e := range ruins {
		if err := ValidarEndpoint(e); !errors.Is(err, ErrEndpoint) {
			t.Errorf("%s deveria ser recusado", e)
		}
	}
}

func TestVAPIDEEnvio(t *testing.T) {
	v, err := VAPIDDaSemente([]byte("chave mestra de teste"), "https://financas.exemplo")
	if err != nil {
		t.Fatal(err)
	}
	v2, _ := VAPIDDaSemente([]byte("chave mestra de teste"), "x")
	if v.Publica != v2.Publica || len(v.Publica) != 87 {
		t.Fatal("a mesma semente dá a mesma chave")
	}
	var recebido *http.Request
	status := http.StatusCreated
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		recebido = r
		w.WriteHeader(status)
	}))
	defer srv.Close()
	// o endereço é o do FCM; a conexão cai no servidor de teste
	tr := srv.Client().Transport.(*http.Transport).Clone()
	tr.DialContext = func(ctx context.Context, rede, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, rede, srv.Listener.Addr().String())
	}
	tr.TLSClientConfig.ServerName = "example.com"
	insc := Inscricao{Endpoint: "https://fcm.googleapis.com/fcm/send/abc", P256dh: "BCVxsr7N_eNgVRqvHtD0zTZsEc6-VV-JvLexhqUzORcxaOzi6-AYWXvTBHm4bjyPjs7Vd8pZGH6SRpkNtoIAiw4",
		Auth: "BTBZMqHH6r4Tts7J_aSIgg"}
	p := &Push{VAPID: v, HTTP: &http.Client{Transport: tr}, Agora: func() time.Time { return time.Unix(1_800_000_000, 0) }}
	if err := p.Enviar(context.Background(), insc, []byte(`{"titulo":"oi"}`)); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(recebido.Header.Get("Authorization"), "vapid t=") || !strings.HasSuffix(recebido.Header.Get("Authorization"), "k="+v.Publica) ||
		recebido.Header.Get("Content-Encoding") != "aes128gcm" || recebido.Header.Get("TTL") == "" || recebido.URL.Path != "/fcm/send/abc" {
		t.Fatalf("cabeçalhos: %v", recebido.Header)
	}
	status = http.StatusGone
	if err := p.Enviar(context.Background(), insc, []byte("x")); !errors.Is(err, ErrInscricaoVencida) {
		t.Fatalf("410: %v", err)
	}
	status = http.StatusInternalServerError
	if err := p.Enviar(context.Background(), insc, []byte("x")); err == nil || errors.Is(err, ErrInscricaoVencida) {
		t.Fatalf("500: %v", err)
	}
	if err := p.Enviar(context.Background(), Inscricao{Endpoint: "https://evil.com/x"}, []byte("x")); !errors.Is(err, ErrEndpoint) {
		t.Fatal("endereço fora da lista")
	}
	if err := p.Enviar(context.Background(), Inscricao{Endpoint: insc.Endpoint, P256dh: "!", Auth: insc.Auth}, []byte("x")); err == nil {
		t.Fatal("chave inválida")
	}
	// assinatura do JWT confere com a chave pública
	tok, err := v.jwt("https://fcm.googleapis.com", time.Unix(1_800_000_000, 0))
	if err != nil {
		t.Fatal(err)
	}
	partes := strings.Split(tok, ".")
	sig, _ := b64.DecodeString(partes[2])
	h := sha256.Sum256([]byte(partes[0] + "." + partes[1]))
	pubBytes, _ := b64.DecodeString(v.Publica)
	x, y := elliptic.Unmarshal(elliptic.P256(), pubBytes) //nolint:staticcheck // só para verificar no teste
	pub := &ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}
	if !ecdsa.Verify(pub, h[:], new(big.Int).SetBytes(sig[:32]), new(big.Int).SetBytes(sig[32:])) {
		t.Fatal("assinatura VAPID inválida")
	}
	claims, _ := b64.DecodeString(partes[1])
	if !strings.Contains(string(claims), `"aud":"https://fcm.googleapis.com"`) || !strings.Contains(string(claims), `"exp":1800043200`) {
		t.Fatalf("claims: %s", claims)
	}
}
