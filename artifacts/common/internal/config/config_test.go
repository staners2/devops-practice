package config

import (
	"testing"
	"time"
)

func TestLoadUsesDefaultsAndRequiredConnections(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://devops:password@postgres/devops_practice")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("KAFKA_BROKERS", "kafka-1:9092,kafka-2:9092")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q, want :8080", cfg.HTTPAddr)
	}
	if cfg.KafkaTopic != "requests.created.v1" {
		t.Fatalf("KafkaTopic = %q", cfg.KafkaTopic)
	}
	if len(cfg.KafkaBrokers) != 2 || cfg.KafkaBrokers[1] != "kafka-2:9092" {
		t.Fatalf("KafkaBrokers = %#v", cfg.KafkaBrokers)
	}
	if cfg.CacheTTL != 30*time.Second || cfg.RequestTimeout != 2*time.Second || cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("unexpected timeouts: %#v", cfg)
	}
	if cfg.Postgres != (Postgres{MaxConns: 5, MinConns: 1, StatementTimeout: time.Second, LockTimeout: 500 * time.Millisecond, IdleInTransactionTimeout: 5 * time.Second}) {
		t.Fatalf("unexpected PostgreSQL defaults: %#v", cfg.Postgres)
	}
}

func TestLoadRejectsInvalidPostgresLimits(t *testing.T) {
	for _, value := range []struct{ name, value string }{
		{"PG_POOL_MAX_CONNS", "0"}, {"PG_POOL_MAX_CONNS", "-1"},
		{"PG_POOL_MAX_CONNS", "2147483648"}, {"PG_POOL_MIN_CONNS", "-1"},
		{"PG_POOL_MIN_CONNS", "6"}, {"PG_STATEMENT_TIMEOUT", "invalid"},
		{"PG_LOCK_TIMEOUT", "0s"}, {"PG_IDLE_IN_TRANSACTION_TIMEOUT", "1ns"},
	} {
		t.Run(value.name+"/"+value.value, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://devops:do-not-log@localhost/devops_practice")
			t.Setenv("REDIS_ADDR", "localhost:6379")
			t.Setenv("KAFKA_BROKERS", "localhost:9092")
			t.Setenv(value.name, value.value)
			if _, err := Load(); err == nil {
				t.Fatal("invalid PostgreSQL limits must fail")
			}
		})
	}
}

func TestLoadRejectsMissingDatabaseURLWithoutLeakingOtherValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "")
	t.Setenv("REDIS_ADDR", "redis.internal:6379")
	t.Setenv("KAFKA_BROKERS", "kafka.internal:9092")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want missing DATABASE_URL error")
	}
	if got := err.Error(); got != "DATABASE_URL is required" {
		t.Fatalf("error = %q", got)
	}
}

func TestLoadRejectsInvalidRedisDB(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://devops:password@postgres/devops_practice")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("REDIS_DB", "not-a-number")
	t.Setenv("KAFKA_BROKERS", "kafka:9092")

	_, err := Load()
	if err == nil || err.Error() != "REDIS_DB must be an integer" {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestLoadRejectsNonPositiveTimeout(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://devops:password@postgres/devops_practice")
	t.Setenv("REDIS_ADDR", "redis:6379")
	t.Setenv("KAFKA_BROKERS", "kafka:9092")
	t.Setenv("REQUEST_TIMEOUT", "0s")

	_, err := Load()
	if err == nil || err.Error() != "REQUEST_TIMEOUT must be positive" {
		t.Fatalf("Load() error = %v", err)
	}
}
