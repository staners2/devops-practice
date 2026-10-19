package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"devops-practice/internal/config"
	"devops-practice/internal/httpapi"
	"devops-practice/internal/kafkabus"
	"devops-practice/internal/observability"
	"devops-practice/internal/postgres"
	"devops-practice/internal/rediscache"
)

func main() {
	logger := observability.Logger(os.Stdout)
	if err := run(logger); err != nil {
		logger.Error("api stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := postgres.New(rootCtx, cfg.DatabaseURL, cfg.Postgres)
	if err != nil {
		return err
	}
	defer store.Close()
	cache := rediscache.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.CacheTTL)
	defer func() { _ = cache.Close() }()
	producer, err := kafkabus.NewProducer(cfg.KafkaBrokers)
	if err != nil {
		return err
	}
	defer producer.Close()

	metrics := observability.New("api")
	metrics.RegisterPostgresPool(store.PoolStats)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpapi.New(store, cache, producer, cfg, logger, metrics).Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	logger.Info("api starting", "address", cfg.HTTPAddr, "metrics_address", cfg.MetricsAddr)
	return observability.Serve(rootCtx, cfg.ShutdownTimeout, server, observability.Server(cfg.MetricsAddr, metrics))
}
