// Package notificar envia as notificações do painel: Web Push (PWA) e Telegram.
package notificar

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Web Push sem biblioteca de terceiros: criptografia do conteúdo pela RFC 8291
// (aes128gcm, RFC 8188) e identificação do servidor pelo VAPID (RFC 8292).

var b64 = base64.RawURLEncoding

// VAPID: a chave do servidor que assina os envios.
type VAPID struct {
	privada *ecdsa.PrivateKey
	Publica string // base64url do ponto não comprimido (vai para o navegador)
	Sujeito string // mailto: ou https: de contato
}

// VAPIDDaSemente deriva a chave de uma semente estável (a chave mestra), para não
// precisar de configuração a mais: HKDF até cair num escalar válido da P-256.
func VAPIDDaSemente(semente []byte, sujeito string) (*VAPID, error) {
	for i := range 16 {
		k, err := hkdf.Key(sha256.New, semente, nil, fmt.Sprintf("financas vapid %d", i), 32)
		if err != nil {
			return nil, err
		}
		priv, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), k)
		if err != nil {
			continue // fora do intervalo da curva (raríssimo): tenta o próximo
		}
		pub, err := priv.PublicKey.Bytes()
		if err != nil {
			return nil, err
		}
		return &VAPID{privada: priv, Publica: b64.EncodeToString(pub), Sujeito: sujeito}, nil
	}
	return nil, errors.New("vapid: não consegui derivar a chave")
}

// jwt assinado (ES256) para a origem do serviço de push.
func (v *VAPID) jwt(audiencia string, agora time.Time) (string, error) {
	cab := b64.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`))
	corpo, err := json.Marshal(map[string]any{"aud": audiencia, "exp": agora.Add(12 * time.Hour).Unix(), "sub": v.Sujeito})
	if err != nil {
		return "", err
	}
	msg := cab + "." + b64.EncodeToString(corpo)
	h := sha256.Sum256([]byte(msg))
	r, s, err := ecdsa.Sign(rand.Reader, v.privada, h[:])
	if err != nil {
		return "", err
	}
	assinatura := make([]byte, 64)
	r.FillBytes(assinatura[:32])
	s.FillBytes(assinatura[32:])
	return msg + "." + b64.EncodeToString(assinatura), nil
}

// Inscricao do navegador (PushSubscription).
type Inscricao struct {
	Endpoint string `json:"endpoint"`
	P256dh   string `json:"p256dh"`
	Auth     string `json:"auth"`
}

// hostsPush: só serviços de push conhecidos (o endereço vem do navegador; sem isso o
// servidor poderia ser usado para chamar qualquer URL — SSRF).
var hostsPush = []string{"fcm.googleapis.com", "updates.push.services.mozilla.com", "push.services.mozilla.com",
	".push.apple.com", ".notify.windows.com"}

// ErrEndpoint: endereço de push fora da lista.
var ErrEndpoint = errors.New("endereço de push não reconhecido")

// ValidarEndpoint: https e host de um serviço de push conhecido.
func ValidarEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" || len(endpoint) > 1000 {
		return ErrEndpoint
	}
	h := strings.ToLower(u.Hostname())
	for _, p := range hostsPush {
		if h == p || (strings.HasPrefix(p, ".") && strings.HasSuffix(h, p)) {
			return nil
		}
	}
	return ErrEndpoint
}

// Cifrar o conteúdo para a inscrição (RFC 8291). sal e chaveEfemera vêm de fora só nos
// testes (vetor da RFC); nil gera novos.
func Cifrar(conteudo []byte, insc Inscricao, sal []byte, efemera *ecdh.PrivateKey) ([]byte, error) {
	uaPub, err := b64.DecodeString(strings.TrimRight(insc.P256dh, "="))
	if err != nil {
		return nil, errors.New("p256dh inválido")
	}
	auth, err := b64.DecodeString(strings.TrimRight(insc.Auth, "="))
	if err != nil || len(auth) != 16 {
		return nil, errors.New("auth inválido")
	}
	uaChave, err := ecdh.P256().NewPublicKey(uaPub)
	if err != nil {
		return nil, errors.New("p256dh inválido")
	}
	if efemera == nil {
		if efemera, err = ecdh.P256().GenerateKey(rand.Reader); err != nil {
			return nil, err
		}
	}
	if sal == nil {
		sal = make([]byte, 16)
		if _, err := rand.Read(sal); err != nil {
			return nil, err
		}
	}
	segredo, err := efemera.ECDH(uaChave)
	if err != nil {
		return nil, err
	}
	asPub := efemera.PublicKey().Bytes()
	info := append(append([]byte("WebPush: info\x00"), uaPub...), asPub...)
	ikm, err := hkdf.Key(sha256.New, segredo, auth, string(info), 32)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Key(sha256.New, ikm, sal, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, sal, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	bloco, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(bloco)
	if err != nil {
		return nil, err
	}
	// um registro só: conteúdo + delimitador 0x02 (último registro)
	cifrado := gcm.Seal(nil, nonce, append(append([]byte{}, conteudo...), 2), nil)
	var cab bytes.Buffer
	cab.Write(sal)
	_ = binary.Write(&cab, binary.BigEndian, uint32(4096))
	cab.WriteByte(byte(len(asPub)))
	cab.Write(asPub)
	return append(cab.Bytes(), cifrado...), nil
}

// ErrInscricaoVencida: o serviço disse que a inscrição não existe mais (404/410).
var ErrInscricaoVencida = errors.New("inscrição de push vencida")

// Push envia uma mensagem (JSON pequeno: título, texto, link) para a inscrição.
type Push struct {
	VAPID *VAPID
	HTTP  *http.Client
	Agora func() time.Time
}

// Enviar a notificação.
func (p *Push) Enviar(ctx context.Context, insc Inscricao, conteudo []byte) error {
	if err := ValidarEndpoint(insc.Endpoint); err != nil {
		return err
	}
	corpo, err := Cifrar(conteudo, insc, nil, nil)
	if err != nil {
		return err
	}
	u, _ := url.Parse(insc.Endpoint)
	agora := time.Now()
	if p.Agora != nil {
		agora = p.Agora()
	}
	token, err := p.VAPID.jwt(u.Scheme+"://"+u.Host, agora)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, insc.Endpoint, bytes.NewReader(corpo))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "vapid t="+token+", k="+p.VAPID.Publica)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("TTL", "86400")
	req.Header.Set("Urgency", "normal")
	cliente := p.HTTP
	if cliente == nil {
		cliente = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := cliente.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	switch {
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		return ErrInscricaoVencida
	case resp.StatusCode >= 300:
		return fmt.Errorf("push: serviço respondeu %d", resp.StatusCode)
	}
	return nil
}
