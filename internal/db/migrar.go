package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/jackc/pgx/v5/stdlib" // driver "pgx" para o goose
	"github.com/pressly/goose/v3"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivermigrate"
)

//go:embed migracoes/*.sql
var migracoes embed.FS

// Migrar aplica as migrações com o papel dono (urlDono), cria ou atualiza o
// papel do app com a senha dada e refaz as permissões. É idempotente.
func Migrar(ctx context.Context, urlDono, senhaApp string) error {
	if senhaApp == "" {
		return fmt.Errorf("APP_DB_PASSWORD vazio")
	}

	sqlDB, err := sql.Open("pgx", urlDono)
	if err != nil {
		return err
	}
	defer sqlDB.Close()

	dir, err := fs.Sub(migracoes, "migracoes")
	if err != nil {
		return err
	}
	provedor, err := goose.NewProvider(goose.DialectPostgres, sqlDB, dir)
	if err != nil {
		return err
	}
	resultados, err := provedor.Up(ctx)
	if err != nil {
		return fmt.Errorf("migrações: %w", err)
	}
	for _, r := range resultados {
		slog.Info("migração aplicada", "arquivo", r.Source.Path, "duração", r.Duration)
	}

	pool, err := pgxpool.New(ctx, urlDono)
	if err != nil {
		return err
	}
	defer pool.Close()

	migrador, err := rivermigrate.New(riverpgxv5.New(pool), nil)
	if err != nil {
		return err
	}
	if _, err := migrador.Migrate(ctx, rivermigrate.DirectionUp, nil); err != nil {
		return fmt.Errorf("migrações da fila: %w", err)
	}

	return pgx.BeginTxFunc(ctx, pool, pgx.TxOptions{}, func(tx pgx.Tx) error {
		return garantirPapelApp(ctx, tx, senhaApp)
	})
}

func garantirPapelApp(ctx context.Context, tx pgx.Tx, senha string) error {
	var banco string
	if err := tx.QueryRow(ctx, "select current_database()").Scan(&banco); err != nil {
		return err
	}
	papel := pgx.Identifier{PapelApp}.Sanitize()
	senhaLiteral, err := literal(ctx, tx, senha)
	if err != nil {
		return err
	}
	comandos := []string{
		fmt.Sprintf(`do $$ begin
			if not exists (select from pg_roles where rolname = '%s') then
				create role %s login;
			end if;
		end $$`, PapelApp, papel),
		fmt.Sprintf("alter role %s with login nosuperuser nobypassrls nocreatedb nocreaterole noinherit password %s", papel, senhaLiteral),
		fmt.Sprintf("grant connect on database %s to %s", pgx.Identifier{banco}.Sanitize(), papel),
		"revoke create on schema public from public",
		fmt.Sprintf("grant usage on schema public to %s", papel),
		fmt.Sprintf("grant select, insert, update, delete on all tables in schema public to %s", papel),
		fmt.Sprintf("grant usage, select on all sequences in schema public to %s", papel),
		// referência global: só leitura
		fmt.Sprintf("revoke insert, update, delete on regras_fiscais, instituicoes from %s", papel),
		// sinais de vida são gravados; auditoria só cresce
		fmt.Sprintf("revoke update, delete on auditoria from %s", papel),
		fmt.Sprintf("revoke all on goose_db_version from %s", papel),
	}
	for _, c := range comandos {
		if _, err := tx.Exec(ctx, c); err != nil {
			return fmt.Errorf("permissões (%.60s...): %w", c, err)
		}
	}
	return nil
}

// literal escapa um texto como literal SQL usando o próprio Postgres.
func literal(ctx context.Context, tx pgx.Tx, s string) (string, error) {
	var out string
	err := tx.QueryRow(ctx, "select quote_literal($1::text)", s).Scan(&out)
	return out, err
}
