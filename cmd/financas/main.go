// Comando financas: um binário para tudo.
//
//	financas serve    API + front na porta 3100
//	financas worker   tarefas em segundo plano
//	financas migrar   aplica migrações (papel dono) e ajusta o papel do app
//	financas saude    confere /api/health local (healthcheck do contêiner)
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
	_ "time/tzdata" // a imagem distroless não tem fusos

	"github.com/yarion1/pi-finance/internal/api"
	"github.com/yarion1/pi-finance/internal/auth"
	"github.com/yarion1/pi-finance/internal/config"
	"github.com/yarion1/pi-finance/internal/cotacoes"
	"github.com/yarion1/pi-finance/internal/cripto"
	"github.com/yarion1/pi-finance/internal/db"
	"github.com/yarion1/pi-finance/internal/openfinance"
	"github.com/yarion1/pi-finance/internal/pluggy"
	"github.com/yarion1/pi-finance/internal/worker"
	"github.com/yarion1/pi-finance/web"
)

// versao é gravada no build: -ldflags "-X main.versao=v0.1.0".
var versao = "dev"

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "uso: financas serve|worker|migrar|saude|versao")
		os.Exit(2)
	}
	ctx, parar := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer parar()

	var err error
	switch os.Args[1] {
	case "serve":
		err = servir(ctx)
	case "worker":
		err = rodarWorker(ctx)
	case "migrar":
		err = migrar(ctx)
	case "saude":
		err = saude()
	case "versao":
		fmt.Println(versao)
	default:
		err = fmt.Errorf("comando desconhecido: %s", os.Args[1])
	}
	if err != nil {
		slog.Error("falhou", "comando", os.Args[1], "erro", err)
		os.Exit(1)
	}
}

func servir(ctx context.Context) error {
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}
	if err := cfg.ExigirServidor(); err != nil {
		return err
	}
	cifrador, err := cripto.CarregarArquivo(cfg.ArquivoChave)
	if err != nil {
		return err
	}
	pool, err := db.Conectar(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	wa, err := auth.NovoWebAuthn(cfg.URLPublica)
	if err != nil {
		return err
	}
	if wa == nil {
		slog.Warn("passkeys desligadas: PUBLIC_URL sem HTTPS ou com IP; o 2FA fica só com o aplicativo autenticador", "url", cfg.URLPublica)
	}
	srv := &api.Servidor{
		Auth:     &auth.Servico{Pool: pool, Cifrador: cifrador, WebAuthn: wa},
		Pool:     pool,
		Cifrador: cifrador,
		Config:   cfg,
		Versao:   versao,
		Front:    web.Dist(),
		OpenFinance: &openfinance.Servico{
			Banco: pool, Cifrador: cifrador, Pluggy: pluggy.Novo(cfg.URLPluggy),
		},
	}
	http := &http.Server{
		Addr:              cfg.Endereco,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
	erros := make(chan error, 1)
	go func() {
		slog.Info("servindo", "endereco", cfg.Endereco, "versao", versao, "url", cfg.URLPublica)
		erros <- http.ListenAndServe()
	}()
	select {
	case err := <-erros:
		return err
	case <-ctx.Done():
	}
	desligar, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	return http.Shutdown(desligar)
}

func rodarWorker(ctx context.Context) error {
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}
	if cfg.DatabaseURL == "" {
		return errors.New("DATABASE_URL não definido")
	}
	cifrador, err := cripto.CarregarArquivo(cfg.ArquivoChave)
	if err != nil {
		return err
	}
	pool, err := db.Conectar(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	of := &openfinance.Servico{Banco: pool, Cifrador: cifrador, Pluggy: pluggy.Novo(cfg.URLPluggy)}
	return worker.Rodar(ctx, pool, versao, of, cotacoes.Padrao(cfg.TokenBrapi))
}

func migrar(ctx context.Context) error {
	cfg, err := config.Carregar()
	if err != nil {
		return err
	}
	if cfg.DatabaseURLMigracao == "" {
		return errors.New("DATABASE_URL_MIGRACAO não definido")
	}
	return db.Migrar(ctx, cfg.DatabaseURLMigracao, cfg.SenhaAppDB)
}

func saude() error {
	endereco := os.Getenv("SAUDE_URL")
	if endereco == "" {
		endereco = "http://127.0.0.1:3100/api/health"
	}
	cliente := &http.Client{Timeout: 5 * time.Second}
	resp, err := cliente.Get(endereco)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health %d", resp.StatusCode)
	}
	return nil
}
