package obs

import (
	"context"
	"time"

	"github.com/exaring/otelpgx"
	"github.com/jackc/pgx/v5"
	"github.com/prometheus/client_golang/prometheus"
)

// DBTracer is a composite pgx.QueryTracer that:
//   - delegates to otelpgx for OTel span creation (one span per sqlc query)
//   - records db_query_duration_seconds Prometheus histogram observations
//
// Wire it into the pgxpool config before creating the pool:
//
//	config.ConnConfig.Tracer = obs.NewDBTracer(metrics.DBDuration)
type DBTracer struct {
	otel     *otelpgx.Tracer
	duration *prometheus.HistogramVec
}

// NewDBTracer creates a DBTracer.
// Parameter capture is intentionally OFF (otelpgx default): query parameters
// include session tokens, user IDs, and PII. Enabling capture via
// otelpgx.WithIncludeQueryParameters() would leak them into the OTLP backend.
func NewDBTracer(hist *prometheus.HistogramVec) *DBTracer {
	return &DBTracer{
		otel:     otelpgx.NewTracer(),
		duration: hist,
	}
}

// dbStartData carries per-query timing state through the pgx tracer context.
type dbStartData struct {
	sql   string
	start time.Time
}

type dbStartKey struct{}

// TraceQueryStart is called at the beginning of every Query, QueryRow, and
// Exec call — including those from sqlc via the database/sql stdlib bridge.
func (t *DBTracer) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	ctx = t.otel.TraceQueryStart(ctx, conn, data)
	return context.WithValue(ctx, dbStartKey{}, dbStartData{
		sql:   data.SQL,
		start: time.Now(),
	})
}

// TraceQueryEnd is called after the query completes (success or error).
func (t *DBTracer) TraceQueryEnd(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryEndData) {
	t.otel.TraceQueryEnd(ctx, conn, data)
	if sd, ok := ctx.Value(dbStartKey{}).(dbStartData); ok {
		query := extractQueryName(sd.sql)
		t.duration.WithLabelValues(query).Observe(time.Since(sd.start).Seconds())
	}
}
