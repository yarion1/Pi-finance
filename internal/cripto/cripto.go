// Package cripto cifra segredos (credenciais, CPF/CNPJ, segredo TOTP) com
// AES-256-GCM. A chave mestra fica em arquivo fora do banco e do backup comum
// (/etc/financas/master.key, 0600).
package cripto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"strings"
)

const prefixo = "v1:"

// Cifrador cifra e decifra textos com uma chave de 32 bytes.
type Cifrador struct {
	aead cipher.AEAD
}

// Novo cria o cifrador a partir da chave crua (32 bytes).
func Novo(chave []byte) (*Cifrador, error) {
	if len(chave) != 32 {
		return nil, fmt.Errorf("chave mestra precisa de 32 bytes, tem %d", len(chave))
	}
	bloco, err := aes.NewCipher(chave)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(bloco)
	if err != nil {
		return nil, err
	}
	return &Cifrador{aead: aead}, nil
}

// CarregarArquivo lê a chave em base64 (saída de `openssl rand -base64 32`).
func CarregarArquivo(caminho string) (*Cifrador, error) {
	info, err := os.Stat(caminho)
	if err != nil {
		return nil, fmt.Errorf("chave mestra: %w", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("chave mestra %s com permissão %o; use chmod 600", caminho, info.Mode().Perm())
	}
	conteudo, err := os.ReadFile(caminho)
	if err != nil {
		return nil, fmt.Errorf("chave mestra: %w", err)
	}
	chave, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(conteudo)))
	if err != nil {
		return nil, fmt.Errorf("chave mestra não está em base64: %w", err)
	}
	return Novo(chave)
}

// Cifrar devolve "v1:" + base64(nonce || texto cifrado). O contexto (ex.: o nome
// da coluna) entra como dado autenticado: um valor copiado para outra coluna não abre.
func (c *Cifrador) Cifrar(texto, contexto string) (string, error) {
	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	selado := c.aead.Seal(nonce, nonce, []byte(texto), []byte(contexto))
	return prefixo + base64.StdEncoding.EncodeToString(selado), nil
}

// ErrCifraInvalida indica valor adulterado, de outra chave ou de outro contexto.
var ErrCifraInvalida = errors.New("valor cifrado inválido")

// Decifrar abre um valor produzido por Cifrar com o mesmo contexto.
func (c *Cifrador) Decifrar(valor, contexto string) (string, error) {
	if !strings.HasPrefix(valor, prefixo) {
		return "", ErrCifraInvalida
	}
	bruto, err := base64.StdEncoding.DecodeString(valor[len(prefixo):])
	if err != nil || len(bruto) < c.aead.NonceSize() {
		return "", ErrCifraInvalida
	}
	nonce, selado := bruto[:c.aead.NonceSize()], bruto[c.aead.NonceSize():]
	texto, err := c.aead.Open(nil, nonce, selado, []byte(contexto))
	if err != nil {
		return "", ErrCifraInvalida
	}
	return string(texto), nil
}
