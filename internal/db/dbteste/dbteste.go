// Package dbteste cria um banco novo e migrado por pacote de teste.
//
// Usa TEST_DATABASE_URL (papel com permissão de criar bancos). Sem a variável,
// os testes que dependem do banco são pulados localmente e falham no CI (CI=true).
package dbteste

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yarion1/pi-finance/internal/db"
)

// Banco é um banco de teste migrado.
type Banco struct {
	Dono *pgxpool.Pool // papel dono (bypass de RLS): só para preparar dados
	App  *pgxpool.Pool // papel financas_app: o que o sistema usa
	URL  string        // URL do papel do app
}

const senhaApp = "senha-de-teste-app"

// Novo cria o banco, migra e devolve os pools. Tudo é removido no fim do teste.
func Novo(t testing.TB) *Banco {
	t.Helper()
	admin := os.Getenv("TEST_DATABASE_URL")
	if admin == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("TEST_DATABASE_URL é obrigatório no CI")
		}
		t.Skip("TEST_DATABASE_URL não definido")
	}
	ctx := context.Background()

	sufixo := make([]byte, 6)
	_, _ = rand.Read(sufixo)
	nome := "financas_t_" + hex.EncodeToString(sufixo)

	conn, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Fatalf("conectar admin: %v", err)
	}
	if _, err := conn.Exec(ctx, "create database "+pgx.Identifier{nome}.Sanitize()); err != nil {
		t.Fatalf("criar banco: %v", err)
	}

	// Os pacotes de teste rodam em paralelo e o papel financas_app é do servidor
	// inteiro: um ALTER ROLE por vez (trava no banco admin, comum a todos).
	if _, err := conn.Exec(ctx, "select pg_advisory_lock(7201002)"); err != nil {
		t.Fatalf("trava de migração: %v", err)
	}
	urlDono := trocarBanco(t, admin, nome, "", "")
	errMigrar := db.Migrar(ctx, urlDono, senhaApp)
	_, _ = conn.Exec(ctx, "select pg_advisory_unlock(7201002)")
	_ = conn.Close(ctx)
	if errMigrar != nil {
		t.Fatalf("migrar: %v", errMigrar)
	}
	urlApp := trocarBanco(t, admin, nome, db.PapelApp, senhaApp)

	dono, err := pgxpool.New(ctx, urlDono)
	if err != nil {
		t.Fatal(err)
	}
	app, err := db.Conectar(ctx, urlApp)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		app.Close()
		dono.Close()
		c, err := pgx.Connect(context.Background(), admin)
		if err != nil {
			return
		}
		defer c.Close(context.Background())
		_, _ = c.Exec(context.Background(), "drop database if exists "+pgx.Identifier{nome}.Sanitize()+" with (force)")
	})
	return &Banco{Dono: dono, App: app, URL: urlApp}
}

func trocarBanco(t testing.TB, base, nome, usuario, senha string) string {
	t.Helper()
	u, err := url.Parse(base)
	if err != nil {
		t.Fatalf("TEST_DATABASE_URL inválida: %v", err)
	}
	u.Path = "/" + nome
	if usuario != "" {
		u.User = url.UserPassword(usuario, senha)
	}
	return u.String()
}

// CriarUsuario insere um usuário direto (sem passar pela autenticação).
func (b *Banco) CriarUsuario(t testing.TB, nome, email string) string {
	t.Helper()
	var id string
	err := b.Dono.QueryRow(context.Background(),
		"insert into usuarios (nome, email, senha_hash) values ($1, $2, 'x') returning id", nome, email).Scan(&id)
	if err != nil {
		t.Fatalf("criar usuário: %v", err)
	}
	return id
}
