package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/base64"
	"strings"
)

// NovoToken devolve um token aleatório (256 bits) em base64url.
func NovoToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand não falha em sistemas suportados
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken é o que vai para o banco; o token em si só existe no cookie ou no link.
func HashToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

var base32Minusculo = base32.NewEncoding("abcdefghijklmnopqrstuvwxyz234567").WithPadding(base32.NoPadding)

// novoCodigoRecuperacao gera "xxxxx-xxxxx" (50 bits).
func novoCodigoRecuperacao() string {
	b := make([]byte, 7)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	s := base32Minusculo.EncodeToString(b)[:10]
	return s[:5] + "-" + s[5:]
}

func normalizarCodigoRecuperacao(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "-", "")
	if len(s) != 10 {
		return ""
	}
	return s[:5] + "-" + s[5:]
}
