package notification

import (
	"errors"
	"os"
	"strings"
	"time"
)

type Config struct {
	GitLabURL       string
	GitLabProjectID string
	GitLabReadToken string
	TelegramToken   string
	TelegramChatID  string
	PollInterval    time.Duration
	StatePath       string
}

func LoadConfig() (Config, error) {
	pollInterval, err := notificationDuration("NOTIFIER_POLL_INTERVAL", 15*time.Second)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		GitLabURL:       notificationValue("GITLAB_API_URL", "https://gitlab.com/api/v4"),
		GitLabProjectID: strings.TrimSpace(os.Getenv("GITLAB_PROJECT_ID")),
		GitLabReadToken: strings.TrimSpace(os.Getenv("GITLAB_READ_TOKEN")),
		TelegramToken:   strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")),
		TelegramChatID:  strings.TrimSpace(os.Getenv("TELEGRAM_CHAT_ID")),
		PollInterval:    pollInterval,
		StatePath:       notificationValue("NOTIFIER_STATE_PATH", "/var/lib/notifier/state.json"),
	}

	for name, value := range map[string]string{
		"GITLAB_PROJECT_ID":  cfg.GitLabProjectID,
		"TELEGRAM_BOT_TOKEN": cfg.TelegramToken,
		"TELEGRAM_CHAT_ID":   cfg.TelegramChatID,
	} {
		if value == "" {
			return Config{}, errors.New(name + " is required")
		}
	}

	return cfg, nil
}

func notificationValue(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func notificationDuration(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.New(name + " must be a duration")
	}
	if value <= 0 {
		return 0, errors.New(name + " must be positive")
	}
	return value, nil
}
