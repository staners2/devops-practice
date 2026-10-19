package observability

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/felixge/httpsnoop"
	"github.com/google/uuid"
)

type correlationKey struct{}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(correlationKey{}).(string)
	return id
}

func (m *Metrics) HTTP(mux *http.ServeMux, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(r.Header.Get("X-Request-ID"))
		if err != nil || id == uuid.Nil {
			id = uuid.New()
		}
		requestID := id.String()
		w.Header().Set("X-Request-ID", requestID)
		r = r.WithContext(context.WithValue(r.Context(), correlationKey{}, requestID))
		method := r.Method
		switch method {
		case "GET", "HEAD", "POST", "PUT", "PATCH", "DELETE", "OPTIONS", "CONNECT", "TRACE":
		default:
			method = "OTHER"
		}
		_, pattern := mux.Handler(r)
		route := "unmatched"
		if pattern != "" {
			route = pattern
			if _, path, ok := strings.Cut(pattern, " "); ok {
				route = path
			}
		}
		m.inFlight.WithLabelValues(method).Inc()
		defer m.inFlight.WithLabelValues(method).Dec()
		response := httpsnoop.CaptureMetrics(mux, w, r)
		m.requests.WithLabelValues(method, route, strconv.Itoa(response.Code)).Inc()
		m.duration.WithLabelValues(method, route).Observe(response.Duration.Seconds())
		logger.Info("http request", "request_id", requestID, "method", method, "route", route,
			"status", response.Code, "duration_seconds", response.Duration.Seconds(), "response_bytes", response.Written)
	})
}
