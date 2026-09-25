package cripto

import (
	"bytes"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCifrarDecifrar(t *testing.T) {
	c, err := Novo(bytes.Repeat([]byte{7}, 32))
	if err != nil {
		t.Fatal(err)
	}
	v1, _ := c.Cifrar("123.456.789-00", "entidades.documento")
	v2, _ := c.Cifrar("123.456.789-00", "entidades.documento")
	if v1 == v2 {
		t.Fatal("mesmo texto deveria gerar cifras diferentes (nonce aleatório)")
	}
	texto, err := c.Decifrar(v1, "entidades.documento")
	if err != nil || texto != "123.456.789-00" {
		t.Fatalf("decifrar: %q, %v", texto, err)
	}
	if _, err := c.Decifrar(v1, "outra.coluna"); !errors.Is(err, ErrCifraInvalida) {
		t.Fatal("contexto diferente deveria falhar")
	}
	adulterado := v1[:len(v1)-2] + "AA"
	if _, err := c.Decifrar(adulterado, "entidades.documento"); !errors.Is(err, ErrCifraInvalida) {
		t.Fatal("valor adulterado deveria falhar")
	}
	outra, _ := Novo(bytes.Repeat([]byte{8}, 32))
	if _, err := outra.Decifrar(v1, "entidades.documento"); !errors.Is(err, ErrCifraInvalida) {
		t.Fatal("outra chave deveria falhar")
	}
}

func TestCarregarArquivo(t *testing.T) {
	dir := t.TempDir()
	caminho := filepath.Join(dir, "master.key")
	chave := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{1}, 32))
	if err := os.WriteFile(caminho, []byte(chave+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := CarregarArquivo(caminho); err == nil {
		t.Fatal("permissão 644 deveria ser recusada")
	}
	if err := os.Chmod(caminho, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := CarregarArquivo(caminho); err != nil {
		t.Fatalf("chave válida: %v", err)
	}
	if _, err := Novo([]byte("curta")); err == nil {
		t.Fatal("chave curta deveria falhar")
	}
}
