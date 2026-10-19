package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"devops-practice/internal/config"
	"devops-practice/internal/observability"
	"devops-practice/internal/postgres"
	"devops-practice/internal/rediscache"
	"devops-practice/internal/request"
	"github.com/google/uuid"
)

type storeStub struct {
	pingErr        error
	getValue       request.Request
	getErr         error
	created        *request.Request
	respectContext bool
}

func (s *storeStub) Ping(context.Context) error { return s.pingErr }
func (s *storeStub) Create(_ context.Context, value request.Request) error {
	s.created = &value
	return nil
}
func (s *storeStub) Get(ctx context.Context, _ uuid.UUID) (request.Request, error) {
	if s.respectContext && ctx.Err() != nil {
		return request.Request{}, ctx.Err()
	}
	return s.getValue, s.getErr
}

type cacheStub struct {
	pingErr  error
	getValue request.Request
	getErr   error
	count    int64
}

func (c *cacheStub) Ping(context.Context) error { return c.pingErr }
func (c *cacheStub) Get(context.Context, uuid.UUID) (request.Request, error) {
	return c.getValue, c.getErr
}
func (c *cacheStub) Set(context.Context, request.Request) error { return nil }
func (c *cacheStub) Delete(context.Context, uuid.UUID) error    { return nil }
func (c *cacheStub) ProcessedCount(context.Context) (int64, error) {
	return c.count, c.pingErr
}

type producerStub struct {
	pingErr    error
	publishErr error
	published  *request.RequestCreatedEvent
}

func (p *producerStub) Ping(context.Context) error { return p.pingErr }
func (p *producerStub) Publish(_ context.Context, _ string, event request.RequestCreatedEvent) error {
	p.published = &event
	return p.publishErr
}

func TestHealthCheckReportsFailedDependency(t *testing.T) {
	server := newTestServer(&storeStub{}, &cacheStub{pingErr: errors.New("redis unavailable")}, &producerStub{})
	recorder := performRequest(server, http.MethodGet, "/api/health-check", nil)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
	want := `{"status":"error","checks":{"postgres":"ok","redis":"error","kafka":"ok"}}`
	if strings.TrimSpace(recorder.Body.String()) != want {
		t.Fatalf("body = %s, want %s", recorder.Body.String(), want)
	}
}

func TestDependencyAndPublicationMetrics(t *testing.T) {
	server := newTestServer(&storeStub{}, &cacheStub{pingErr: errors.New("redis unavailable")}, &producerStub{publishErr: errors.New("publish failed")})
	performRequest(server, "GET", "/readyz", nil)
	performRequest(server, "POST", "/api/requests", strings.NewReader(`{"title":"practice"}`))
	w := httptest.NewRecorder()
	server.metrics.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	for _, want := range []string{`dependency="redis"} 0`, `dependency="postgres"} 1`, `devops_practice_events_published_total{result="error"} 1`} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestLivenessDoesNotCheckDependencies(t *testing.T) {
	server := newTestServer(
		&storeStub{pingErr: errors.New("postgres unavailable")},
		&cacheStub{pingErr: errors.New("redis unavailable")},
		&producerStub{pingErr: errors.New("kafka unavailable")},
	)
	recorder := performRequest(server, http.MethodGet, "/livez", nil)

	if recorder.Code != http.StatusOK || strings.TrimSpace(recorder.Body.String()) != `{"status":"ok"}` {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestReadinessReportsFailedDependency(t *testing.T) {
	server := newTestServer(&storeStub{}, &cacheStub{pingErr: errors.New("redis unavailable")}, &producerStub{})
	recorder := performRequest(server, http.MethodGet, "/readyz", nil)

	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
	want := `{"status":"error","checks":{"postgres":"ok","redis":"error","kafka":"ok"}}`
	if strings.TrimSpace(recorder.Body.String()) != want {
		t.Fatalf("body = %s, want %s", recorder.Body.String(), want)
	}
}

func TestCreateRequestPersistsAndPublishes(t *testing.T) {
	store := &storeStub{}
	producer := &producerStub{}
	server := newTestServer(store, &cacheStub{}, producer)
	recorder := performRequest(server, http.MethodPost, "/api/requests", strings.NewReader(`{"title":"  Prepare release  "}`))

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if store.created == nil || store.created.Title != "Prepare release" || store.created.Status != request.StatusCreated {
		t.Fatalf("created = %#v", store.created)
	}
	if producer.published == nil || producer.published.RequestID != store.created.ID {
		t.Fatalf("published = %#v", producer.published)
	}
}

func TestCreateRequestReturns503WhenKafkaPublishFailsAfterInsert(t *testing.T) {
	store := &storeStub{}
	server := newTestServer(store, &cacheStub{}, &producerStub{publishErr: errors.New("kafka unavailable")})
	recorder := performRequest(server, http.MethodPost, "/api/requests", strings.NewReader(`{"title":"publish failure"}`))

	if recorder.Code != http.StatusServiceUnavailable || store.created == nil {
		t.Fatalf("status = %d created = %#v body = %s", recorder.Code, store.created, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), `"code":"event_publish_failed"`) {
		t.Fatalf("body = %s", recorder.Body.String())
	}
}

func TestCreateRequestRejectsUnknownField(t *testing.T) {
	server := newTestServer(&storeStub{}, &cacheStub{}, &producerStub{})
	recorder := performRequest(server, http.MethodPost, "/api/requests", strings.NewReader(`{"title":"valid","admin":true}`))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestCreateRequestRejectsOversizedBody(t *testing.T) {
	server := newTestServer(&storeStub{}, &cacheStub{}, &producerStub{})
	body := `{"title":"` + strings.Repeat("a", 1100) + `"}`
	recorder := performRequest(server, http.MethodPost, "/api/requests", strings.NewReader(body))

	if recorder.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", recorder.Code)
	}
}

func TestGetRequestFallsBackToPostgresWhenRedisFails(t *testing.T) {
	id := uuid.MustParse("609a50aa-f017-4134-8f47-14a08a533f61")
	want := request.Request{ID: id, Title: "cached later", Status: request.StatusCreated}
	server := newTestServer(
		&storeStub{getValue: want},
		&cacheStub{getErr: errors.New("redis unavailable")},
		&producerStub{},
	)
	recorder := performRequest(server, http.MethodGet, "/api/requests/"+id.String(), nil)

	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), id.String()) {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestGetRequestGetsFreshPostgresTimeoutAfterRedisTimesOut(t *testing.T) {
	id := uuid.MustParse("609a50aa-f017-4134-8f47-14a08a533f61")
	want := request.Request{ID: id, Title: "database fallback", Status: request.StatusCreated}
	cache := &deadlineCacheStub{}
	server := New(
		&storeStub{getValue: want, respectContext: true}, cache, &producerStub{},
		config.Config{KafkaTopic: "requests.created.v1", RequestTimeout: 10 * time.Millisecond},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		observability.New("api"),
	)
	recorder := performRequest(server, http.MethodGet, "/api/requests/"+id.String(), nil)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s, want PostgreSQL fallback 200", recorder.Code, recorder.Body.String())
	}
}

func TestGetRequestMapsMissingRowTo404(t *testing.T) {
	id := uuid.MustParse("609a50aa-f017-4134-8f47-14a08a533f61")
	server := newTestServer(
		&storeStub{getErr: postgres.ErrNotFound},
		&cacheStub{getErr: rediscache.ErrCacheMiss},
		&producerStub{},
	)
	recorder := performRequest(server, http.MethodGet, "/api/requests/"+id.String(), nil)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
}

func newTestServer(store Store, cache Cache, producer Producer) *Server {
	return New(store, cache, producer, config.Config{
		KafkaTopic:     "requests.created.v1",
		RequestTimeout: time.Second,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)), observability.New("api"))
}

func performRequest(server *Server, method, path string, body *strings.Reader) *httptest.ResponseRecorder {
	var reader io.Reader
	if body != nil {
		reader = body
	} else {
		reader = bytes.NewReader(nil)
	}
	request := httptest.NewRequest(method, path, reader)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder
}

type deadlineCacheStub struct{}

func (deadlineCacheStub) Ping(context.Context) error { return nil }
func (deadlineCacheStub) Get(ctx context.Context, _ uuid.UUID) (request.Request, error) {
	<-ctx.Done()
	return request.Request{}, ctx.Err()
}
func (deadlineCacheStub) Set(context.Context, request.Request) error { return nil }
func (deadlineCacheStub) Delete(context.Context, uuid.UUID) error    { return nil }
func (deadlineCacheStub) ProcessedCount(context.Context) (int64, error) {
	return 0, nil
}
