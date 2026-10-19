package observability

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPNormalizesRoutesAndCorrelatesRequests(t *testing.T) {
	m := New("api")
	var logs bytes.Buffer
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/requests/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("ok"))
	})
	h := m.HTTP(mux, slog.New(slog.NewJSONHandler(&logs, nil)))
	id := "64ed00ff-c2f2-49ef-acdd-2042c2146b39"
	r := httptest.NewRequest("GET", "/api/requests/"+id+"?secret=hidden", nil)
	r.Header.Set("X-Request-ID", id)
	r.Header.Set("Authorization", "secret-authorization")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 202 || w.Body.String() != "ok" || w.Header().Get("X-Request-ID") != id {
		t.Fatalf("response changed: %d %s %v", w.Code, w.Body, w.Header())
	}
	var entry map[string]any
	if err := json.Unmarshal(logs.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	if entry["request_id"] != id || entry["route"] != "/api/requests/{id}" || entry["status"] != float64(202) || entry["response_bytes"] != float64(2) {
		t.Fatalf("missing request context: %v", entry)
	}
	if strings.Contains(logs.String(), "hidden") || strings.Contains(logs.String(), "secret-authorization") {
		t.Fatal("log contains request secrets")
	}
	metrics := httptest.NewRecorder()
	m.Handler().ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(metrics.Body.String(), `devops_practice_http_requests_total{method="GET",route="/api/requests/{id}",status_code="202"} 1`) || strings.Contains(metrics.Body.String(), id) {
		t.Fatalf("wrong metrics: %s", metrics.Body)
	}
}

func TestUnknownMethodsAndPathsHaveBoundedLabels(t *testing.T) {
	m := New("api")
	h := m.HTTP(http.NewServeMux(), slog.New(slog.NewJSONHandler(io.Discard, nil)))
	for _, path := range []string{"/unknown/one", "/unknown/two"} {
		r := httptest.NewRequest("CUSTOM-"+path[9:], path, nil)
		r.Header.Set("X-Request-ID", "invalid-id\t")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Header().Get("X-Request-ID") == "invalid-id\t" || w.Header().Get("X-Request-ID") == "" {
			t.Fatal("invalid correlation ID retained")
		}
	}
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(w.Body.String(), `method="OTHER",route="unmatched",status_code="404"} 2`) {
		t.Fatalf("unbounded labels: %s", w.Body)
	}
}

type plainWriter struct{ *bytes.Buffer }

func (w plainWriter) Header() http.Header { return http.Header{} }
func (w plainWriter) WriteHeader(int)     {}

func TestHTTPPreservesOptionalWriterInterfaces(t *testing.T) {
	for _, writer := range []http.ResponseWriter{plainWriter{new(bytes.Buffer)}, httptest.NewRecorder()} {
		mux := http.NewServeMux()
		mux.HandleFunc("GET /", func(w http.ResponseWriter, _ *http.Request) {
			_, wantFlush := writer.(http.Flusher)
			_, gotFlush := w.(http.Flusher)
			_, wantHijack := writer.(http.Hijacker)
			_, gotHijack := w.(http.Hijacker)
			_, wantPush := writer.(http.Pusher)
			_, gotPush := w.(http.Pusher)
			_, wantRead := writer.(io.ReaderFrom)
			_, gotRead := w.(io.ReaderFrom)
			if gotFlush != wantFlush || gotHijack != wantHijack || gotPush != wantPush || gotRead != wantRead {
				t.Fatalf("optional interfaces changed for %T", writer)
			}
		})
		New("api").HTTP(mux, slog.New(slog.NewJSONHandler(io.Discard, nil))).ServeHTTP(writer, httptest.NewRequest("GET", "/", nil))
	}
}
