package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"devops-practice/internal/config"
	"devops-practice/internal/observability"
	"devops-practice/internal/postgres"
	"devops-practice/internal/rediscache"
	"devops-practice/internal/request"
	"github.com/google/uuid"
)

type Store interface {
	Ping(context.Context) error
	Create(context.Context, request.Request) error
	Get(context.Context, uuid.UUID) (request.Request, error)
}

type Cache interface {
	Ping(context.Context) error
	Get(context.Context, uuid.UUID) (request.Request, error)
	Set(context.Context, request.Request) error
	Delete(context.Context, uuid.UUID) error
	ProcessedCount(context.Context) (int64, error)
}

type Producer interface {
	Ping(context.Context) error
	Publish(context.Context, string, request.RequestCreatedEvent) error
}

type healthChecks struct {
	Postgres string `json:"postgres"`
	Redis    string `json:"redis"`
	Kafka    string `json:"kafka"`
}

type healthResponse struct {
	Status string       `json:"status"`
	Checks healthChecks `json:"checks"`
}

type Server struct {
	store    Store
	cache    Cache
	producer Producer
	cfg      config.Config
	logger   *slog.Logger
	metrics  *observability.Metrics
	handler  http.Handler
}

func New(store Store, cache Cache, producer Producer, cfg config.Config, logger *slog.Logger, metrics *observability.Metrics) *Server {
	server := &Server{store: store, cache: cache, producer: producer, cfg: cfg, logger: logger, metrics: metrics}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", server.liveness)
	mux.HandleFunc("GET /readyz", server.readiness)
	mux.HandleFunc("GET /api/health-check", server.healthCheck)
	mux.HandleFunc("POST /api/requests", server.createRequest)
	mux.HandleFunc("GET /api/requests/{id}", server.getRequest)
	mux.HandleFunc("GET /api/stats", server.getStats)
	server.handler = metrics.HTTP(mux, logger)
	return server
}

func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Status string `json:"status"`
	}{Status: "ok"})
}

func (s *Server) healthCheck(w http.ResponseWriter, r *http.Request) {
	s.readiness(w, r)
}

func (s *Server) readiness(w http.ResponseWriter, r *http.Request) {
	response := s.dependencyHealth(r.Context())
	status := http.StatusOK
	if response.Status != "ok" {
		status = http.StatusServiceUnavailable
	}
	writeJSON(w, status, response)
}

func (s *Server) dependencyHealth(ctx context.Context) healthResponse {
	response := healthResponse{
		Status: "ok",
		Checks: healthChecks{Postgres: "ok", Redis: "ok", Kafka: "ok"},
	}

	if err := withTimeout(ctx, s.cfg.RequestTimeout, s.store.Ping); err != nil {
		response.Status, response.Checks.Postgres = "error", "error"
	}
	s.metrics.Dependency(observability.Postgres, response.Checks.Postgres == "ok")
	if err := withTimeout(ctx, s.cfg.RequestTimeout, s.cache.Ping); err != nil {
		response.Status, response.Checks.Redis = "error", "error"
	}
	s.metrics.Dependency(observability.Redis, response.Checks.Redis == "ok")
	if err := withTimeout(ctx, s.cfg.RequestTimeout, s.producer.Ping); err != nil {
		response.Status, response.Checks.Kafka = "error", "error"
	}
	s.metrics.Dependency(observability.Kafka, response.Checks.Kafka == "ok")
	return response
}

func (s *Server) createRequest(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	var input request.CreateInput
	if err := decoder.Decode(&input); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "тело запроса слишком большое")
			return
		}
		writeError(w, http.StatusBadRequest, "invalid_request", "некорректный JSON")
		return
	}
	if err := ensureSingleJSONValue(decoder); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_request", "ожидается один JSON-объект")
		return
	}

	input.Title = strings.TrimSpace(input.Title)
	if count := utf8.RuneCountInString(input.Title); count < 1 || count > 200 {
		writeError(w, http.StatusBadRequest, "invalid_title", "название должно содержать от 1 до 200 символов")
		return
	}

	now := time.Now().UTC()
	value := request.Request{
		ID: uuid.New(), Title: input.Title, Status: request.StatusCreated,
		CreatedAt: now, UpdatedAt: now,
	}
	storeCtx, cancelStore := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	err := s.store.Create(storeCtx, value)
	cancelStore()
	if err != nil {
		s.logger.Error("create request", "request_id", value.ID, "error", err)
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "сервис временно недоступен")
		return
	}
	cacheCtx, cancelCache := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	err = s.cache.Delete(cacheCtx, value.ID)
	cancelCache()
	if err != nil {
		s.logger.Warn("invalidate request cache", "request_id", value.ID, "error", err)
	}
	event := request.RequestCreatedEvent{
		Version: request.EventVersion, Type: request.EventTypeRequestCreated,
		RequestID: value.ID, OccurredAt: now,
	}
	producerCtx, cancelProducer := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	err = s.producer.Publish(producerCtx, s.cfg.KafkaTopic, event)
	cancelProducer()
	s.metrics.Published(err == nil)
	if err != nil {
		s.logger.Error("publish request event", "request_id", value.ID, "error", err)
		writeError(w, http.StatusServiceUnavailable, "event_publish_failed", "событие не опубликовано")
		return
	}
	s.logger.Info("request created", "request_id", value.ID, "http_request_id", observability.RequestID(r.Context()), "status", value.Status, "event_type", event.Type)
	writeJSON(w, http.StatusCreated, value)
}

func (s *Server) getRequest(w http.ResponseWriter, r *http.Request) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_id", "некорректный идентификатор")
		return
	}
	cacheCtx, cancelCache := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	value, err := s.cache.Get(cacheCtx, id)
	cancelCache()
	if err == nil {
		writeJSON(w, http.StatusOK, value)
		return
	}
	if !errors.Is(err, rediscache.ErrCacheMiss) {
		s.logger.Warn("read request cache", "request_id", id, "error", err)
	}

	storeCtx, cancelStore := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	value, err = s.store.Get(storeCtx, id)
	cancelStore()
	if errors.Is(err, postgres.ErrNotFound) {
		writeError(w, http.StatusNotFound, "not_found", "заявка не найдена")
		return
	}
	if err != nil {
		s.logger.Error("read request", "request_id", id, "error", err)
		writeError(w, http.StatusServiceUnavailable, "database_unavailable", "сервис временно недоступен")
		return
	}
	cacheWriteCtx, cancelCacheWrite := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	err = s.cache.Set(cacheWriteCtx, value)
	cancelCacheWrite()
	if err != nil {
		s.logger.Warn("write request cache", "request_id", id, "error", err)
	}
	writeJSON(w, http.StatusOK, value)
}

func (s *Server) getStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	defer cancel()
	count, err := s.cache.ProcessedCount(ctx)
	if err != nil {
		s.logger.Error("read processed count", "error", err)
		writeError(w, http.StatusServiceUnavailable, "redis_unavailable", "сервис временно недоступен")
		return
	}
	writeJSON(w, http.StatusOK, struct {
		Processed int64 `json:"processed"`
	}{Processed: count})
}

func ensureSingleJSONValue(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return errors.New("additional JSON value")
}

func withTimeout(parent context.Context, timeout time.Duration, check func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()
	return check(ctx)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}{Error: struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}{Code: code, Message: message}})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
