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
	DatabaseURL     string
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

func Load() (Config, error) {
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
		HTTPAddr:        value("HTTP_ADDR", ":8080"),
		DatabaseURL:     strings.TrimSpace(os.Getenv("DATABASE_URL")),
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
