package obs

import (
	"strings"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds the three core Prometheus instrumentation vectors.
// All label values must be bounded: use route patterns, HTTP methods, and
// status codes only. Never use raw URLs, user IDs, or resource IDs as label
// values — unbounded cardinality will cause Prometheus memory exhaustion.
type Metrics struct {
	// HTTPRequests counts completed HTTP requests.
	HTTPRequests *prometheus.CounterVec
	// HTTPDuration measures HTTP request latency end-to-end.
	HTTPDuration *prometheus.HistogramVec
	// DBDuration measures individual DB query latency by sqlc method name.
	DBDuration *prometheus.HistogramVec
}

// httpBuckets covers the expected SSR latency range (5 ms – 5 s).
var httpBuckets = []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0, 2.5, 5.0}

// dbBuckets covers fast DB query latency (1 ms – 1 s).
var dbBuckets = []float64{0.001, 0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1.0}

// NewMetrics creates and registers the three core metric vectors against reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		HTTPRequests: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "http_requests_total",
				Help: "Total number of HTTP requests completed, by method, route pattern, and status code.",
			},
			[]string{"method", "route", "status"},
		),
		HTTPDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "http_request_duration_seconds",
				Help:    "HTTP request latency in seconds, by method, route pattern, and status code.",
				Buckets: httpBuckets,
			},
			[]string{"method", "route", "status"},
		),
		DBDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "db_query_duration_seconds",
				Help:    "DB query latency in seconds, by sqlc method name.",
				Buckets: dbBuckets,
			},
			[]string{"query"},
		),
	}
	reg.MustRegister(m.HTTPRequests, m.HTTPDuration, m.DBDuration)
	return m
}

// extractQueryName parses the sqlc comment header at the top of generated SQL
// constants ("-- name: MethodName :one\n…") and returns the method name.
// Falls back to the first word of the SQL (e.g. "SELECT") if no comment is found.
func extractQueryName(sql string) string {
	const prefix = "-- name: "
	if strings.HasPrefix(sql, prefix) {
		rest := sql[len(prefix):]
		if i := strings.IndexByte(rest, ' '); i > 0 {
			return rest[:i]
		}
	}
	if i := strings.IndexByte(sql, ' '); i > 0 {
		return sql[:i]
	}
	return sql
}
