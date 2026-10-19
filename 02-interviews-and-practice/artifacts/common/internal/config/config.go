package config

import (
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	HTTPAddr        string
	MetricsAddr     string
	DatabaseURL     string
	Postgres        Postgres
	RedisAddr       string
	RedisPassword   string
	RedisDB         int
	KafkaBrokers    []string
	KafkaTopic      string
	KafkaGroup      string
	CacheTTL        time.Duration
	RequestTimeout  time.Duration
	ShutdownTimeout time.Duration
}

type Postgres struct {
	MaxConns                 int32
	MinConns                 int32
	StatementTimeout         time.Duration
	LockTimeout              time.Duration
	IdleInTransactionTimeout time.Duration
}

func Load() (Config, error) {
	maxConns, err := integer("PG_POOL_MAX_CONNS", 5)
	if err != nil {
		return Config{}, err
	}
	minConns, err := integer("PG_POOL_MIN_CONNS", 1)
	if err != nil {
		return Config{}, err
	}
	if maxConns <= 0 || maxConns > 2147483647 || minConns < 0 || minConns > maxConns {
		return Config{}, errors.New("PostgreSQL pool requires 0 <= min <= max and 0 < max <= 2147483647")
	}
	statementTimeout, err := duration("PG_STATEMENT_TIMEOUT", time.Second)
	if err != nil {
		return Config{}, err
	}
	lockTimeout, err := duration("PG_LOCK_TIMEOUT", 500*time.Millisecond)
	if err != nil {
		return Config{}, err
	}
	idleTimeout, err := duration("PG_IDLE_IN_TRANSACTION_TIMEOUT", 5*time.Second)
	if err != nil {
		return Config{}, err
	}
	for _, timeout := range []time.Duration{statementTimeout, lockTimeout, idleTimeout} {
		if timeout < time.Millisecond || timeout.Milliseconds() > 2147483647 {
			return Config{}, errors.New("PostgreSQL timeouts must be between 1ms and 2147483647ms")
		}
	}
	redisDB, err := integer("REDIS_DB", 0)
	if err != nil {
		return Config{}, err
	}
	cacheTTL, err := duration("CACHE_TTL", 30*time.Second)
	if err != nil {
		return Config{}, err
	}
	requestTimeout, err := duration("REQUEST_TIMEOUT", 2*time.Second)
	if err != nil {
		return Config{}, err
	}
	shutdownTimeout, err := duration("SHUTDOWN_TIMEOUT", 10*time.Second)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		HTTPAddr:    value("HTTP_ADDR", ":8080"),
		MetricsAddr: value("METRICS_ADDR", ":9090"),
		DatabaseURL: strings.TrimSpace(os.Getenv("DATABASE_URL")),
		Postgres: Postgres{
			MaxConns: int32(maxConns), MinConns: int32(minConns),
			StatementTimeout: statementTimeout, LockTimeout: lockTimeout,
			IdleInTransactionTimeout: idleTimeout,
		},
		RedisAddr:       strings.TrimSpace(os.Getenv("REDIS_ADDR")),
		RedisPassword:   os.Getenv("REDIS_PASSWORD"),
		RedisDB:         redisDB,
		KafkaBrokers:    splitNonEmpty(os.Getenv("KAFKA_BROKERS")),
		KafkaTopic:      value("KAFKA_TOPIC", "requests.created.v1"),
		KafkaGroup:      value("KAFKA_GROUP", "request-worker-v1"),
		CacheTTL:        cacheTTL,
		RequestTimeout:  requestTimeout,
		ShutdownTimeout: shutdownTimeout,
	}

	if cfg.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	if cfg.RedisAddr == "" {
		return Config{}, errors.New("REDIS_ADDR is required")
	}
	if len(cfg.KafkaBrokers) == 0 {
		return Config{}, errors.New("KAFKA_BROKERS is required")
	}
	return cfg, nil
}

func value(name, fallback string) string {
	if raw := strings.TrimSpace(os.Getenv(name)); raw != "" {
		return raw
	}
	return fallback
}

func integer(name string, fallback int) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New(name + " must be an integer")
	}
	return parsed, nil
}

func duration(name string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, errors.New(name + " must be a duration")
	}
	if parsed <= 0 {
		return 0, errors.New(name + " must be positive")
	}
	return parsed, nil
}

func splitNonEmpty(raw string) []string {
	var values []string
	for _, value := range strings.Split(raw, ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}
