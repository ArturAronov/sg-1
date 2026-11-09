package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ArturAronov/sg-1/sg-1/backend/internal/config"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/httpapi"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/identity"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/platform"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/secrets"
	"github.com/ArturAronov/sg-1/sg-1/backend/internal/storage"
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
	if command != "serve" && command != "migrate" && command != "seed-dev" && command != "worker" && command != "gateway" && command != "storage-init" {
		logger.Error("usage: sg1 [serve|migrate|seed-dev]")
		return errors.New("unknown command")
	}
	c, err := config.Load()
	if err != nil {
		logger.Error("invalid configuration", "reason", err.Error())
		return err
	}
	if (command == "serve" || command == "worker" || command == "gateway") && c.Environment != "development" && c.Runtime != "aws" {
		return errors.New("production requires the authenticated AWS runtime")
	}
	if command == "seed-dev" && c.Environment != "development" {
		logger.Error("seed-dev is forbidden outside development")
		return errors.New("development only")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var objects *storage.S3
	if bucket := os.Getenv("SG1_S3_BUCKET"); bucket != "" {
		region := os.Getenv("AWS_REGION")
		if region == "" {
			region = "us-east-1"
		}
		objects, err = storage.New(ctx, bucket, os.Getenv("SG1_S3_ENDPOINT"), region, os.Getenv("SG1_S3_ACCESS_KEY"), os.Getenv("SG1_S3_SECRET"))
		if err != nil {
			return err
		}
	}
	if command == "storage-init" {
		if c.Environment != "development" || objects == nil {
			return errors.New("local storage initialization is not configured")
		}
		for range 30 {
			attempt, cancel := context.WithTimeout(ctx, 5*time.Second)
			err = objects.Initialize(attempt)
			cancel()
			if err == nil {
				logger.Info("private development object bucket ready")
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(time.Second):
			}
		}
		return errors.New("object storage did not become ready")
	}
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
	if c.Environment == "production" {
		s.ProjectLimit = 30
	}
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
	api := &httpapi.API{Store: s, Logger: logger, Development: c.Environment == "development", DevToken: c.DevToken, DevUserID: store.DevUserID}
	if !api.Development {
		bootstrap := map[string]bool{}
		for _, subject := range c.BootstrapSubjects {
			bootstrap[subject] = true
		}
		api.Identity = &identity.Authenticator{Store: s, Verifier: &identity.Verifier{Region: c.Region, SignerARN: c.ALBARN, Issuer: c.CognitoIssuer, ClientID: c.CognitoClient}, BootstrapSubjects: bootstrap}
	}
	api.Files = objects
	if c.CredentialsKey != "" {
		api.Vault, err = secrets.NewVault(c.CredentialsKey)
		if err != nil {
			return err
		}
	} else {
		logger.Warn("secure LLM credential storage disabled; configure SG1_CREDENTIALS_KEY")
	}
	if (c.Runtime == "docker" || c.Runtime == "aws") && api.Vault != nil {
		signing := sha256.Sum256([]byte("sg1-gateway/v1:" + c.CredentialsKey))
		api.Platform = &platform.Service{Store: s, Vault: api.Vault, AppOrigin: c.AppOrigin, DatabaseHost: "sg1-postgres-1", SigningKey: signing[:], Provider: &platform.ProviderClient{}, GatewayURL: "http://gateway:8081"}
		if c.Runtime == "aws" {
			api.Platform.Cloud, err = platform.NewAWSRuntime(ctx, s, objects)
			if err != nil {
				return err
			}
			api.Platform.DatabaseTLS = true
			u, e := url.Parse(c.ProjectDatabaseURL)
			if e != nil {
				return e
			}
			api.Platform.DatabaseHost = u.Hostname()
			if command == "worker" {
				api.Platform.ProjectPool, err = pgxpool.New(ctx, c.ProjectDatabaseURL)
				if err != nil {
					return err
				}
				defer api.Platform.ProjectPool.Close()
				if err = api.Platform.ProjectPool.Ping(startup); err != nil {
					return err
				}
			}
		}
	}
	if command == "worker" {
		if api.Platform == nil {
			return errors.New("runtime is not configured")
		}
		if c.Runtime == "docker" {
			api.Platform.Docker = &platform.Docker{NetworkPrefix: "sg1-project-", GatewayContainer: "sg1-gateway-1", PostgresContainer: "sg1-postgres-1"}
		}
		return api.Platform.RunWorker(ctx, logger)
	}
	server := &http.Server{Addr: c.ListenAddr, Handler: api.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	if command == "gateway" {
		if api.Platform == nil {
			return errors.New("runtime is not configured")
		}
		server.Addr = "0.0.0.0:8081"
		server.Handler = api.Platform.Gateway(c.PublicURL)
		server.ReadTimeout = 0
		server.WriteTimeout = 0
	}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdown)
	}()
	if api.Development {
		logger.Warn("development authentication enabled; do not expose this service publicly", "listen", c.ListenAddr)
	} else {
		logger.Info("AWS employee authentication enabled", "listen", c.ListenAddr)
	}
	if err := server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
