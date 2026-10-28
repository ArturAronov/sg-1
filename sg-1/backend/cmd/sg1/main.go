package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/config"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/httpapi"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/secrets"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		// Detailed startup errors are intentionally not emitted; connection errors may contain secrets.
		logger.Error("command failed; check configuration, database connectivity, and migration status")
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	command := "serve"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}
	if command != "serve" && command != "migrate" && command != "seed-dev" {
		logger.Error("usage: sg1 [serve|migrate|seed-dev]")
		return errors.New("unknown command")
	}
	c, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "reason", err.Error())
		return err
	}
	if command == "serve" && c.Environment != "development" {
		logger.Error("production server is disabled until SSO is implemented")
		return errors.New("SSO unavailable")
	}
	if command == "seed-dev" && c.Environment != "development" {
		logger.Error("seed-dev is forbidden outside development")
		return errors.New("development only")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	poolConfig, err := pgxpool.ParseConfig(c.DatabaseURL)
	if err != nil {
		return err
	}
	poolConfig.MaxConns = 10
	poolConfig.ConnConfig.ConnectTimeout = 5 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return err
	}
	defer pool.Close()
	startup, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := pool.Ping(startup); err != nil {
		return err
	}
	s := &store.Store{Pool: pool}
	switch command {
	case "migrate":
		if err := store.Migrate(startup, pool); err != nil {
			return err
		}
		logger.Info("database migrations applied")
		return nil
	case "seed-dev":
		if err := s.SeedDevelopment(startup); err != nil {
			return err
		}
		logger.Info("local development user and department ready")
		return nil
	}
	api := &httpapi.API{Store: s, Logger: logger, Development: true, DevToken: c.DevToken, DevUserID: store.DevUserID}
	if c.CredentialsKey != "" {
		api.Vault, err = secrets.NewVault(c.CredentialsKey)
		if err != nil {
			return err
		}
	} else {
		logger.Warn("secure LLM credential storage disabled; configure SG1_CREDENTIALS_KEY")
	}
	server := &http.Server{Addr: c.ListenAddr, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	logger.Warn("development authentication enabled; do not expose this service publicly", "listen", c.ListenAddr)
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
