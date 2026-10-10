package observability

import (
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Dependency string

const (
	Postgres Dependency = "postgres"
	Redis    Dependency = "redis"
	Kafka    Dependency = "kafka"
)

type Result string

const (
	Success   Result = "success"
	Error     Result = "error"
	Discarded Result = "discarded"
)

type Metrics struct {
	registry       *prometheus.Registry
	requests       *prometheus.CounterVec
	duration       *prometheus.HistogramVec
	inFlight       *prometheus.GaugeVec
	dependencies   *prometheus.GaugeVec
	published      *prometheus.CounterVec
	workerRecords  *prometheus.CounterVec
	workerDuration *prometheus.HistogramVec
	lastSuccess    prometheus.Gauge
}

func (m *Metrics) RegisterPostgresPool(stats func() *pgxpool.Stat) {
	const prefix = "devops_practice_postgres_pool_"
	m.registry.MustRegister(
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: prefix + "max_connections", Help: "Configured pool connection limit."}, func() float64 { return float64(stats().MaxConns()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: prefix + "acquired_connections", Help: "Connections currently borrowed from the pool."}, func() float64 { return float64(stats().AcquiredConns()) }),
		prometheus.NewGaugeFunc(prometheus.GaugeOpts{Name: prefix + "idle_connections", Help: "Idle pool connections."}, func() float64 { return float64(stats().IdleConns()) }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Name: prefix + "acquire_duration_seconds_total", Help: "Cumulative connection acquisition duration, not SQL execution time."}, func() float64 { return stats().AcquireDuration().Seconds() }),
		prometheus.NewCounterFunc(prometheus.CounterOpts{Name: prefix + "canceled_acquires_total", Help: "Connection acquisitions canceled while waiting."}, func() float64 { return float64(stats().CanceledAcquireCount()) }),
	)
}

func New(component string) *Metrics {
	const prefix = "devops_practice_"
	m := &Metrics{
		registry:       prometheus.NewRegistry(),
		requests:       prometheus.NewCounterVec(prometheus.CounterOpts{Name: prefix + "http_requests_total", Help: "Completed HTTP requests."}, []string{"method", "route", "status_code"}),
		duration:       prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: prefix + "http_request_duration_seconds", Help: "HTTP request duration.", Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2, 5, 10}}, []string{"method", "route"}),
		inFlight:       prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: prefix + "http_requests_in_flight", Help: "Active HTTP requests."}, []string{"method"}),
		dependencies:   prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: prefix + "dependency_up", Help: "Last dependency check: 1 healthy, 0 unavailable."}, []string{"dependency"}),
		published:      prometheus.NewCounterVec(prometheus.CounterOpts{Name: prefix + "events_published_total", Help: "Kafka publication attempts."}, []string{"result"}),
		workerRecords:  prometheus.NewCounterVec(prometheus.CounterOpts{Name: prefix + "worker_records_total", Help: "Kafka processing attempts, including retries."}, []string{"result"}),
		workerDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{Name: prefix + "worker_processing_duration_seconds", Help: "Duration of a processing attempt."}, []string{"result"}),
		lastSuccess:    prometheus.NewGauge(prometheus.GaugeOpts{Name: prefix + "worker_last_success_timestamp_seconds", Help: "Unix timestamp of last committed valid event; zero before first success."}),
	}
	m.registry.MustRegister(collectors.NewGoCollector(), collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		m.requests, m.duration, m.inFlight, m.dependencies, m.published, m.workerRecords, m.workerDuration, m.lastSuccess)
	if component == "api" {
		m.published.WithLabelValues(string(Success))
		m.published.WithLabelValues(string(Error))
	}
	if component == "worker" {
		for _, result := range []Result{Success, Error, Discarded} {
			m.workerRecords.WithLabelValues(string(result))
		}
	}
	return m
}

func (m *Metrics) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /-/healthy", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	return mux
}

func (m *Metrics) Dependency(dependency Dependency, up bool) {
	switch dependency {
	case Postgres, Redis, Kafka:
	default:
		panic("unknown metric dependency")
	}
	value := 0.0
	if up {
		value = 1
	}
	m.dependencies.WithLabelValues(string(dependency)).Set(value)
}

func (m *Metrics) Published(ok bool) {
	result := Error
	if ok {
		result = Success
	}
	m.published.WithLabelValues(string(result)).Inc()
}

func (m *Metrics) RecordWorker(result Result, elapsed time.Duration) {
	switch result {
	case Success, Error, Discarded:
	default:
		panic("unknown worker result")
	}
	m.workerRecords.WithLabelValues(string(result)).Inc()
	m.workerDuration.WithLabelValues(string(result)).Observe(elapsed.Seconds())
	if result == Success {
		m.lastSuccess.Set(float64(time.Now().Unix()))
	}
}
