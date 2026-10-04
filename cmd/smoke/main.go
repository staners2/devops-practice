package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"devops-practice/internal/config"
	"devops-practice/internal/kafkabus"
	"devops-practice/internal/rediscache"
	"devops-practice/internal/request"
	"github.com/google/uuid"
)

type smoke struct {
	baseURL  string
	client   *http.Client
	cache    *rediscache.Cache
	producer *kafkabus.Producer
	topic    string
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(); err != nil {
		logger.Error("smoke failed", "error", err)
		os.Exit(1)
	}
	logger.Info("smoke passed")
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	baseURL := strings.TrimRight(os.Getenv("API_BASE_URL"), "/")
	if baseURL == "" {
		return errors.New("API_BASE_URL is required")
	}

	cache := rediscache.New(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB, cfg.CacheTTL)
	defer func() { _ = cache.Close() }()
	producer, err := kafkabus.NewProducer(cfg.KafkaBrokers)
	if err != nil {
		return err
	}
	defer producer.Close()

	check := &smoke{
		baseURL: baseURL, client: &http.Client{Timeout: 5 * time.Second},
		cache: cache, producer: producer, topic: cfg.KafkaTopic,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	return check.run(ctx)
}

func (s *smoke) run(ctx context.Context) error {
	if err := s.waitForHealth(ctx); err != nil {
		return err
	}
	initialCount, err := s.stats(ctx)
	if err != nil {
		return err
	}

	created, err := s.create(ctx, "module 00 smoke request")
	if err != nil {
		return err
	}
	processed, err := s.waitForProcessed(ctx, created.ID)
	if err != nil {
		return err
	}

	if err := s.cache.Delete(ctx, created.ID); err != nil {
		return fmt.Errorf("delete cache before PostgreSQL fallback check: %w", err)
	}
	var fallback request.Request
	if err := s.doJSON(ctx, http.MethodGet, "/api/requests/"+created.ID.String(), nil, http.StatusOK, &fallback); err != nil {
		return fmt.Errorf("PostgreSQL fallback: %w", err)
	}
	if fallback.ID != created.ID || fallback.Status != request.StatusProcessed {
		return fmt.Errorf("PostgreSQL fallback returned unexpected request: %#v", fallback)
	}

	count, err := s.stats(ctx)
	if err != nil {
		return err
	}
	if count != initialCount+1 {
		return fmt.Errorf("processed count = %d, want %d", count, initialCount+1)
	}
	if err := s.cache.Set(ctx, processed); err != nil {
		return fmt.Errorf("set duplicate-delivery sentinel: %w", err)
	}
	if err := s.producer.Publish(ctx, s.topic, request.RequestCreatedEvent{
		Version: request.EventVersion, Type: request.EventTypeRequestCreated,
		RequestID: created.ID, OccurredAt: time.Now().UTC(),
	}); err != nil {
		return fmt.Errorf("publish duplicate event: %w", err)
	}
	if err := s.waitForCacheDeletion(ctx, created.ID); err != nil {
		return err
	}
	countAfterDuplicate, err := s.stats(ctx)
	if err != nil {
		return err
	}
	if countAfterDuplicate != count {
		return fmt.Errorf("duplicate changed processed count from %d to %d", count, countAfterDuplicate)
	}

	if err := s.validationChecks(ctx); err != nil {
		return err
	}
	return nil
}

func (s *smoke) waitForHealth(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var response struct {
			Status string `json:"status"`
		}
		if err := s.doJSON(ctx, http.MethodGet, "/api/health-check", nil, http.StatusOK, &response); err == nil && response.Status == "ok" {
			return nil
		}
		select {
		case <-ctx.Done():
			return errors.New("health-check did not become ready")
		case <-ticker.C:
		}
	}
}

func (s *smoke) create(ctx context.Context, title string) (request.Request, error) {
	var created request.Request
	err := s.doJSON(ctx, http.MethodPost, "/api/requests", map[string]string{"title": title}, http.StatusCreated, &created)
	if err != nil {
		return request.Request{}, fmt.Errorf("create request: %w", err)
	}
	if created.ID.String() == "" || created.Title != title || created.Status != request.StatusCreated {
		return request.Request{}, fmt.Errorf("unexpected created request: %#v", created)
	}
	return created, nil
}

func (s *smoke) waitForProcessed(ctx context.Context, id uuid.UUID) (request.Request, error) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		var value request.Request
		if err := s.doJSON(ctx, http.MethodGet, "/api/requests/"+id.String(), nil, http.StatusOK, &value); err == nil && value.Status == request.StatusProcessed {
			return value, nil
		}
		select {
		case <-ctx.Done():
			return request.Request{}, errors.New("request did not become processed")
		case <-ticker.C:
		}
	}
}

func (s *smoke) waitForCacheDeletion(ctx context.Context, id uuid.UUID) error {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		_, err := s.cache.Get(ctx, id)
		if errors.Is(err, rediscache.ErrCacheMiss) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read duplicate-delivery sentinel: %w", err)
		}
		select {
		case <-ctx.Done():
			return errors.New("worker did not consume duplicate event")
		case <-ticker.C:
		}
	}
}

func (s *smoke) stats(ctx context.Context) (int64, error) {
	var response struct {
		Processed int64 `json:"processed"`
	}
	if err := s.doJSON(ctx, http.MethodGet, "/api/stats", nil, http.StatusOK, &response); err != nil {
		return 0, fmt.Errorf("read stats: %w", err)
	}
	return response.Processed, nil
}

func (s *smoke) validationChecks(ctx context.Context) error {
	checks := []struct {
		name   string
		method string
		path   string
		body   any
		status int
	}{
		{name: "missing request", method: http.MethodGet, path: "/api/requests/00000000-0000-0000-0000-000000000001", status: http.StatusNotFound},
		{name: "invalid request id", method: http.MethodGet, path: "/api/requests/not-a-uuid", status: http.StatusBadRequest},
		{name: "empty title", method: http.MethodPost, path: "/api/requests", body: map[string]string{"title": "  "}, status: http.StatusBadRequest},
		{name: "unknown field", method: http.MethodPost, path: "/api/requests", body: map[string]any{"title": "valid", "admin": true}, status: http.StatusBadRequest},
		{name: "oversized body", method: http.MethodPost, path: "/api/requests", body: map[string]string{"title": strings.Repeat("a", 1100)}, status: http.StatusRequestEntityTooLarge},
	}
	for _, check := range checks {
		if err := s.doJSON(ctx, check.method, check.path, check.body, check.status, nil); err != nil {
			return fmt.Errorf("%s: %w", check.name, err)
		}
	}
	return nil
}

func (s *smoke) doJSON(ctx context.Context, method, path string, body any, wantStatus int, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	response, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	if err != nil {
		return err
	}
	if response.StatusCode != wantStatus {
		return fmt.Errorf("%s %s status = %d, want %d, body = %s", method, path, response.StatusCode, wantStatus, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			return fmt.Errorf("decode %s %s response: %w", method, path, err)
		}
	}
	return nil
}
