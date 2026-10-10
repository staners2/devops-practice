package observability

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsExposeResultsWithoutIdentifiers(t *testing.T) {
	m := New("worker")
	m.RecordWorker(Success, time.Millisecond)
	m.RecordWorker(Error, time.Millisecond)
	m.RecordWorker(Discarded, time.Millisecond)
	m.Dependency(Redis, false)
	m.Published(false)
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	for _, want := range []string{
		`devops_practice_worker_records_total{result="success"} 1`,
		`devops_practice_worker_records_total{result="error"} 1`,
		`devops_practice_worker_records_total{result="discarded"} 1`,
		`devops_practice_dependency_up{dependency="redis"} 0`,
		`devops_practice_events_published_total{result="error"} 1`,
		`devops_practice_worker_last_success_timestamp_seconds`, `go_goroutines`, `process_start_time_seconds`,
	} {
		if !strings.Contains(w.Body.String(), want) {
			t.Errorf("missing %s", want)
		}
	}
	other := New("worker")
	w = httptest.NewRecorder()
	other.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(w.Body.String(), "devops_practice_worker_last_success_timestamp_seconds 0") {
		t.Fatal("registries share state")
	}
}

func TestMetricLabelsExcludeSensitiveFields(t *testing.T) {
	m := New("api")
	m.Dependency(Postgres, true)
	m.Published(true)
	m.RecordWorker(Success, time.Millisecond)
	families, err := m.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		for _, sample := range family.GetMetric() {
			for _, label := range sample.GetLabel() {
				switch label.GetName() {
				case "request_id", "uuid", "url", "raw_url", "query", "title", "error", "offset", "partition", "pod_uid":
					t.Errorf("sensitive label %s on %s", label.GetName(), family.GetName())
				}
			}
		}
	}
}
