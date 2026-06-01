package obs

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// LevelVar is the runtime-adjustable log level for the process.
// External code (e.g. a debug endpoint) may call LevelVar.Set(slog.LevelDebug).
var LevelVar = &slog.LevelVar{}

// Init configures the global slog logger and OTel TracerProvider:
//
//   - JSON output to stdout, UTC timestamps, PII redaction
//   - Log level from LOG_LEVEL env var (DEBUG/INFO/WARN/ERROR; default INFO)
//   - "service.name" attr on every log record (OTel semantic convention)
//   - OTLP/gRPC TracerProvider reading OTEL_EXPORTER_OTLP_* from env
//   - W3C TraceContext propagator
//
// If OTEL_EXPORTER_OTLP_ENDPOINT is not set, tracing is disabled (no-op
// provider) and a warning is logged. This allows local dev without a collector.
//
// The shutdown func flushes pending spans with a 5-second timeout and must be
// called before the process exits (wire it to SIGTERM).
func Init(ctx context.Context, serviceName string) (func(), error) {
	LevelVar.Set(parseLevel(os.Getenv("LOG_LEVEL")))

	jsonHandler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level:       LevelVar,
		ReplaceAttr: replaceAttr,
	})

	slog.SetDefault(slog.New(NewRedactHandler(jsonHandler)).With(
		slog.String("service.name", serviceName), //nolint:sloglint
	))

	if os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT") == "" {
		slog.Warn("OTLP endpoint not set — tracing disabled")
		return func() {}, nil
	}

	exp, err := otlptracegrpc.New(ctx)
	if err != nil {
		return func() {}, fmt.Errorf("otlp tracer: %w", err)
	}

	res, err := resource.New(ctx,
		resource.WithFromEnv(),      // reads OTEL_SERVICE_NAME, OTEL_RESOURCE_ATTRIBUTES
		resource.WithProcess(),      // PID, executable name
		resource.WithTelemetrySDK(), // SDK name, language, version
	)
	if err != nil {
		// Non-fatal: partial resource is still usable.
		slog.Warn("resource detection partial", "error", err)
	}

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp),
		sdktrace.WithResource(res),
		// Sampler is intentionally not set here; OTEL_TRACES_SAMPLER env var
		// controls it (platform sets parentbased_traceidratio). Without
		// OTEL_TRACES_SAMPLER_ARG the ratio defaults to 1.0 (100% root spans).
	)

	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	shutdown := func() { //nolint:contextcheck // startup ctx is already cancelled at this point; use a fresh drain context
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			slog.Error("tracer provider shutdown", "error", err)
		}
	}

	return shutdown, nil
}

// replaceAttr normalises the top-level timestamp to UTC.
func replaceAttr(groups []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey && len(groups) == 0 {
		a.Value = slog.TimeValue(a.Value.Time().In(time.UTC))
	}
	return a
}

// parseLevel converts a LOG_LEVEL string to slog.Level.
// Returns slog.LevelInfo on empty input or parse failure.
func parseLevel(s string) slog.Level {
	var l slog.Level
	if err := l.UnmarshalText([]byte(s)); err != nil {
		return slog.LevelInfo
	}
	return l
}
