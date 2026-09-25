// Package worker roda as tarefas em segundo plano sobre a fila do Postgres (River).
// Sinal de vida a cada minuto, limpeza de sessões vencidas e, de hora em hora, a
// sincronização do Open Finance de quem está há mais de 20 h sem sincronizar.
package worker

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/yarion1/pi-finance/internal/auth"
	"github.com/yarion1/pi-finance/internal/openfinance"
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

// OpenFinanceArgs sincroniza o Meu Pluggy de quem está atrasado.
type OpenFinanceArgs struct{}

func (OpenFinanceArgs) Kind() string { return "open_finance" }

type sincronizarOpenFinance struct {
	river.WorkerDefaults[OpenFinanceArgs]
	pool *pgxpool.Pool
	of   *openfinance.Servico
}

// Timeout: um ano de extrato de várias pessoas passa do padrão de 1 minuto.
func (t *sincronizarOpenFinance) Timeout(*river.Job[OpenFinanceArgs]) time.Duration {
	return 30 * time.Minute
}

func (t *sincronizarOpenFinance) Work(ctx context.Context, _ *river.Job[OpenFinanceArgs]) error {
	return SincronizarAtrasados(ctx, t.pool, t.of, time.Now().Add(-20*time.Hour))
}

// SincronizarAtrasados roda a sincronização de cada pessoa cuja última foi antes de
// `antes`. O erro de uma pessoa fica gravado na conexão dela e não para as outras.
func SincronizarAtrasados(ctx context.Context, pool *pgxpool.Pool, of *openfinance.Servico, antes time.Time) error {
	linhas, err := pool.Query(ctx, "select * from app_usuarios_para_sincronizar($1)", antes)
	if err != nil {
		return err
	}
	usuarios, err := pgx.CollectRows(linhas, pgx.RowTo[string])
	if err != nil {
		return err
	}
	for _, u := range usuarios {
		rel, err := of.Sincronizar(ctx, u)
		if err != nil {
			slog.Warn("open finance: sincronização falhou", "usuario", u, "erro", err)
			continue
		}
		slog.Info("open finance: sincronizado", "usuario", u, "novas", rel.Novas, "divergencias", rel.Divergencias, "erros", len(rel.Erros))
	}
	return nil
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
func Rodar(ctx context.Context, pool *pgxpool.Pool, versao string, of *openfinance.Servico) error {
	trabalhadores := river.NewWorkers()
	river.AddWorker(trabalhadores, &sinalVida{pool: pool, versao: versao})
	river.AddWorker(trabalhadores, &limpeza{pool: pool})
	river.AddWorker(trabalhadores, &sincronizarOpenFinance{pool: pool, of: of})

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
			river.NewPeriodicJob(river.PeriodicInterval(time.Hour),
				func() (river.JobArgs, *river.InsertOpts) { return OpenFinanceArgs{}, nil },
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
