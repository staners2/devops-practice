package notification

import (
	"strings"
	"testing"
	"time"
)

func TestLoadConfigDefaults(t *testing.T) {
	t.Setenv("GITLAB_PROJECT_ID", "group/devops-practice")
	t.Setenv("TELEGRAM_BOT_TOKEN", "12345678:test-token")
	t.Setenv("TELEGRAM_CHAT_ID", "-1001234567890")

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GitLabURL != "https://gitlab.com/api/v4" {
		t.Fatalf("GitLabURL = %q", cfg.GitLabURL)
	}
	if cfg.PollInterval != 15*time.Second {
		t.Fatalf("PollInterval = %s", cfg.PollInterval)
	}
	if cfg.StatePath != "/var/lib/notifier/state.json" {
		t.Fatalf("StatePath = %q", cfg.StatePath)
	}
}

func TestLoadConfigRequiresSecretsWithoutLeakingTheirValues(t *testing.T) {
	t.Setenv("GITLAB_PROJECT_ID", "group/devops-practice")
	t.Setenv("TELEGRAM_BOT_TOKEN", "")
	t.Setenv("TELEGRAM_CHAT_ID", "sensitive-chat")

	_, err := LoadConfig()
	if err == nil {
		t.Fatal("LoadConfig() error = nil")
	}
	if err.Error() != "TELEGRAM_BOT_TOKEN is required" {
		t.Fatalf("error = %q", err)
	}
	if strings.Contains(err.Error(), "sensitive-chat") {
		t.Fatal("error leaked environment value")
	}
}

func TestLoadConfigRejectsNonPositivePollInterval(t *testing.T) {
	t.Setenv("GITLAB_PROJECT_ID", "group/devops-practice")
	t.Setenv("TELEGRAM_BOT_TOKEN", "12345678:test-token")
	t.Setenv("TELEGRAM_CHAT_ID", "-1001234567890")
	t.Setenv("NOTIFIER_POLL_INTERVAL", "0s")

	_, err := LoadConfig()
	if err == nil || err.Error() != "NOTIFIER_POLL_INTERVAL must be positive" {
		t.Fatalf("LoadConfig() error = %v", err)
	}
}
