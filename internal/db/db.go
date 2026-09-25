// Package db concentra o acesso ao PostgreSQL.
//
// O app conecta com o papel financas_app, que não é dono das tabelas e não tem
// BYPASSRLS: toda leitura de dados financeiros passa pelas políticas RLS. O
// usuário da requisição entra na transação por ComUsuario; sem ele, as políticas
// não liberam nenhuma linha.
package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PapelApp é o papel do Postgres usado pelo servidor e pelo worker.
const PapelApp = "financas_app"

// Conectar abre o pool do app.
func Conectar(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("DATABASE_URL inválida: %w", err)
	}
	cfg.MaxConns = 10
	cfg.ConnConfig.RuntimeParams["timezone"] = "America/Sao_Paulo"
	cfg.ConnConfig.RuntimeParams["application_name"] = "financas"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres indisponível: %w", err)
	}
	return pool, nil
}

// Iniciador é o que ComUsuario precisa: um pool ou uma conexão.
type Iniciador interface {
	BeginTx(ctx context.Context, opts pgx.TxOptions) (pgx.Tx, error)
}

// ErrSemUsuario indica chamada sem usuário autenticado.
var ErrSemUsuario = errors.New("db: usuário vazio")

// ComUsuario roda fn numa transação em que app.usuario_id é o usuário dado.
// É o único caminho para ler ou gravar dados financeiros.
func ComUsuario(ctx context.Context, banco Iniciador, usuarioID string, fn func(pgx.Tx) error) error {
	if usuarioID == "" {
		return ErrSemUsuario
	}
	return pgx.BeginTxFunc(ctx, banco, pgx.TxOptions{}, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "select set_config('app.usuario_id', $1, true)", usuarioID); err != nil {
			return err
		}
		return fn(tx)
	})
}
