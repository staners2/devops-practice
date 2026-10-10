package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"devops-practice/internal/config"
	"devops-practice/internal/kafkabus"
	"devops-practice/internal/observability"
	"devops-practice/internal/postgres"
	"devops-practice/internal/processor"
	"devops-practice/internal/rediscache"
)

func main() {
	logger := observability.Logger(os.Stdout)
	if err := run(logger); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	store, err := postgres.New(ctx, cfg.DatabaseURL, cfg.Postgres)
	if err != nil {
		return err
	}
	defer store.Close()
	cache := rediscache.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.CacheTTL)
	defer func() { _ = cache.Close() }()
	consumer, err := kafkabus.NewConsumer(cfg.KafkaBrokers, cfg.KafkaTopic, cfg.KafkaGroup)
	if err != nil {
		return err
	}
	defer consumer.Close()

	logger.Info("worker started", "topic", cfg.KafkaTopic, "group", cfg.KafkaGroup)
	metrics := observability.New("worker")
	metrics.RegisterPostgresPool(store.PoolStats)
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	workerErrors := make(chan error, 1)
	serverErrors := make(chan error, 1)
	go func() { workerErrors <- processor.New(store, cache, consumer, logger, metrics).Run(workCtx) }()
	go func() {
		serverErrors <- observability.Serve(workCtx, cfg.ShutdownTimeout, observability.Server(cfg.MetricsAddr, metrics))
	}()
	select {
	case err := <-workerErrors:
		cancel()
		return errors.Join(err, <-serverErrors)
	case err := <-serverErrors:
		cancel()
		timer := time.NewTimer(cfg.ShutdownTimeout)
		defer timer.Stop()
		select {
		case workerErr := <-workerErrors:
			return errors.Join(err, workerErr)
		case <-timer.C:
			return errors.Join(err, context.DeadlineExceeded)
		}
	}
}
