package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"devops-practice/internal/config"
	"devops-practice/internal/kafkabus"
	"devops-practice/internal/postgres"
	"devops-practice/internal/processor"
	"devops-practice/internal/rediscache"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
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

	store, err := postgres.New(ctx, cfg.DatabaseURL)
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
	return processor.New(store, cache, consumer, logger).Run(ctx)
}
