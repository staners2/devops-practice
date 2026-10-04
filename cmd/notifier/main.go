package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"devops-practice/internal/notification"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(logger); err != nil {
		logger.Error("notifier stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := notification.LoadConfig()
	if err != nil {
		return err
	}
	state, err := notification.OpenState(cfg.StatePath)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Second}
	service := notification.NewService(
		notification.NewGitLabSource(client, cfg),
		notification.NewTelegramSender(client, cfg.TelegramToken, cfg.TelegramChatID),
		state,
		logger,
	)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return service.Run(ctx, cfg.PollInterval)
}
