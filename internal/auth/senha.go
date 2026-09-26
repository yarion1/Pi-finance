package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parâmetros do argon2id. 64 MiB por hash cabe folgado nos 512 MB do serviço web
// no Pi; os parâmetros ficam gravados no hash, então podem subir depois.
const (
	argonMemoria   = 64 * 1024
	argonIteracoes = 3
	argonParalelo  = 2
	argonTamSal    = 16
	argonTamChave  = 32
	SenhaTamMinimo = 12
	SenhaTamMaximo = 128
)

// vagasArgon limita os hashes simultâneos: cada um usa 64 MiB, e uma rajada de logins
// (mesmo vinda de vários IPs) não pode estourar a memória do serviço.
var vagasArgon = make(chan struct{}, 3)

func argonIDKey(senha, sal []byte, iteracoes, memoria uint32, paralelo uint8, tam uint32) []byte {
	vagasArgon <- struct{}{}
	defer func() { <-vagasArgon }()
	return argon2.IDKey(senha, sal, iteracoes, memoria, paralelo, tam)
}

// HashSenha devolve o hash no formato PHC ($argon2id$v=19$m=...,t=...,p=...$sal$hash).
func HashSenha(senha string) (string, error) {
	sal := make([]byte, argonTamSal)
	if _, err := rand.Read(sal); err != nil {
		return "", err
	}
	chave := argonIDKey([]byte(senha), sal, argonIteracoes, argonMemoria, argonParalelo, argonTamChave)
	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoria, argonIteracoes, argonParalelo, b64.EncodeToString(sal), b64.EncodeToString(chave)), nil
}

var errHashInvalido = errors.New("hash de senha inválido")

// ConferirSenha compara em tempo constante.
func ConferirSenha(senha, hash string) (bool, error) {
	partes := strings.Split(hash, "$")
	if len(partes) != 6 || partes[1] != "argon2id" {
		return false, errHashInvalido
	}
	var versao int
	if _, err := fmt.Sscanf(partes[2], "v=%d", &versao); err != nil || versao != argon2.Version {
		return false, errHashInvalido
	}
	var m uint32
	var t uint32
	var p uint8
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false, errHashInvalido
	}
	b64 := base64.RawStdEncoding
	sal, err := b64.DecodeString(partes[4])
	if err != nil {
		return false, errHashInvalido
	}
	esperado, err := b64.DecodeString(partes[5])
	if err != nil {
		return false, errHashInvalido
	}
	if m == 0 || m > 256*1024 || t == 0 || t > 16 || p == 0 || len(esperado) == 0 || len(esperado) > 64 {
		return false, errHashInvalido
	}
	obtido := argonIDKey([]byte(senha), sal, t, m, p, uint32(len(esperado)))
	return subtle.ConstantTimeCompare(obtido, esperado) == 1, nil
}

// hashFalso é conferido quando o e-mail não existe, para o tempo de resposta
// não revelar quem tem conta.
var hashFalso, _ = HashSenha("senha-que-ninguem-usa-" + fmt.Sprint(argonMemoria))
