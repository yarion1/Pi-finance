// Package worker roda as tarefas em segundo plano sobre a fila do Postgres (River).
// Fase 0: sinal de vida a cada minuto e limpeza de sessões vencidas.
package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/yarion1/pi-finance/internal/auth"
)

// SinalVidaArgs grava o sinal de vida lido pelo /api/health.
type SinalVidaArgs struct{}

func (SinalVidaArgs) Kind() string { return "sinal_vida" }

type sinalVida struct {
	river.WorkerDefaults[SinalVidaArgs]
	pool   *pgxpool.Pool
	versao string
}

func (t *sinalVida) Work(ctx context.Context, _ *river.Job[SinalVidaArgs]) error {
	return GravarSinal(ctx, t.pool, "worker", map[string]any{"versao": t.versao})
}

// LimpezaArgs apaga sessões, desafios e convites vencidos.
type LimpezaArgs struct{}

func (LimpezaArgs) Kind() string { return "limpeza" }

type limpeza struct {
	river.WorkerDefaults[LimpezaArgs]
	pool *pgxpool.Pool
}

func (t *limpeza) Work(ctx context.Context, _ *river.Job[LimpezaArgs]) error {
	return auth.Limpar(ctx, t.pool)
}

// GravarSinal registra que um serviço está vivo.
func GravarSinal(ctx context.Context, pool *pgxpool.Pool, servico string, dados map[string]any) error {
	if dados == nil {
		dados = map[string]any{}
	}
	_, err := pool.Exec(ctx, `insert into sinais_vida (servico, visto_em, dados) values ($1, now(), $2)
		on conflict (servico) do update set visto_em = now(), dados = excluded.dados`, servico, dados)
	return err
}

// Rodar inicia o worker e bloqueia até o contexto acabar.
func Rodar(ctx context.Context, pool *pgxpool.Pool, versao string) error {
	trabalhadores := river.NewWorkers()
	river.AddWorker(trabalhadores, &sinalVida{pool: pool, versao: versao})
	river.AddWorker(trabalhadores, &limpeza{pool: pool})

	cliente, err := river.NewClient(riverpgxv5.New(pool), &river.Config{
		Logger:  slog.Default(),
		Queues:  map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 4}},
		Workers: trabalhadores,
		PeriodicJobs: []*river.PeriodicJob{
			river.NewPeriodicJob(river.PeriodicInterval(time.Minute),
				func() (river.JobArgs, *river.InsertOpts) { return SinalVidaArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true}),
			river.NewPeriodicJob(river.PeriodicInterval(time.Hour),
				func() (river.JobArgs, *river.InsertOpts) { return LimpezaArgs{}, nil },
				&river.PeriodicJobOpts{RunOnStart: true}),
		},
	})
	if err != nil {
		return err
	}
	// o primeiro sinal sai já, sem esperar a eleição de líder da fila
	if err := GravarSinal(ctx, pool, "worker", map[string]any{"versao": versao}); err != nil {
		return err
	}
	if err := cliente.Start(ctx); err != nil {
		return err
	}
	slog.Info("worker iniciado", "versao", versao)
	<-ctx.Done()
	parar, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelar()
	return cliente.Stop(parar)
}
