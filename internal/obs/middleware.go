package obs

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// ── context keys ────────────────────────────────────────────────────────────

type requestIDKey struct{}

// RequestIDFromContext returns the request ID stored in ctx, if any.
func RequestIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(requestIDKey{}).(string)
	return v, ok
}

// ── SourceIPMiddleware / UserAgentMiddleware ─────────────────────────────────

type sourceIPKey struct{}
type userAgentKey struct{}

// SourceIPMiddleware extracts the client IP and injects it into context.
// X-Forwarded-For is only trusted when the connecting IP is in trustedCIDRs.
// Pass nil or empty slice to always use RemoteAddr (safe default, no proxy).
// Place this inside TraceMiddleware so the span is already active.
func SourceIPMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractIP(r)
		ua := r.Header.Get("User-Agent")
		ctx := context.WithValue(r.Context(), sourceIPKey{}, ip)
		ctx = context.WithValue(ctx, userAgentKey{}, ua)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// SourceIPMiddlewareWithTrustedCIDRs is like SourceIPMiddleware but only trusts
// X-Forwarded-For when the connecting IP matches one of the trusted CIDR ranges.
func SourceIPMiddlewareWithTrustedCIDRs(trustedCIDRs []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := extractIPWithTrust(r, trustedCIDRs)
			ua := r.Header.Get("User-Agent")
			ctx := context.WithValue(r.Context(), sourceIPKey{}, ip)
			ctx = context.WithValue(ctx, userAgentKey{}, ua)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SourceIPFromContext returns the client IP injected by SourceIPMiddleware.
func SourceIPFromContext(ctx context.Context) string {
	v, _ := ctx.Value(sourceIPKey{}).(string)
	return v
}

// UserAgentFromContext returns the User-Agent injected by SourceIPMiddleware.
func UserAgentFromContext(ctx context.Context) string {
	v, _ := ctx.Value(userAgentKey{}).(string)
	return v
}

// extractIP returns the client IP from RemoteAddr, ignoring X-Forwarded-For.
// Use extractIPWithTrust when the app is behind a trusted reverse proxy.
func extractIP(r *http.Request) string {
	return remoteAddr(r)
}

// extractIPWithTrust returns the first X-Forwarded-For token when the
// connecting IP is in trustedCIDRs; otherwise falls back to RemoteAddr.
func extractIPWithTrust(r *http.Request, trustedCIDRs []*net.IPNet) string {
	remote := remoteAddr(r)
	xff := r.Header.Get("X-Forwarded-For")
	if len(trustedCIDRs) > 0 && xff != "" {
		remoteIP := net.ParseIP(remote)
		for _, cidr := range trustedCIDRs {
			if remoteIP != nil && cidr.Contains(remoteIP) {
				// Connecting IP is a trusted proxy — trust the first XFF token.
				if i := strings.IndexByte(xff, ','); i >= 0 {
					return strings.TrimSpace(xff[:i])
				}
				return strings.TrimSpace(xff)
			}
		}
	}
	return remote
}

func remoteAddr(r *http.Request) string {
	host := r.RemoteAddr
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}

// routeContainer is a mutable pointer shared via context between
// HTTPMetricsMiddleware (which injects it) and InstrumentedMux (which fills
// it). Using a pointer allows InstrumentedMux to write through context value
// copies created by r.WithContext calls in intermediate middleware.
type routeContainer struct{ pattern string }
type routeKey struct{}

func withRouteContainer(ctx context.Context) (context.Context, *routeContainer) {
	rc := &routeContainer{}
	return context.WithValue(ctx, routeKey{}, rc), rc
}

// ── InstrumentedMux ─────────────────────────────────────────────────────────

// InstrumentedMux wraps *http.ServeMux so that each dispatch uses
// mux.Handler to resolve the matched pattern and injects it as an otelhttp
// route tag. otelhttp will then rename the span to "{method} {pattern}" after
// the handler returns.
//
// No changes to module Register functions are required.
type InstrumentedMux struct {
	mux *http.ServeMux
}

// NewInstrumentedMux wraps mux with route-pattern tagging for OTel spans.
func NewInstrumentedMux(mux *http.ServeMux) *InstrumentedMux {
	return &InstrumentedMux{mux: mux}
}

func (m *InstrumentedMux) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// mux.Handler does NOT set path values on r (per Go docs: "Handler does
	// not modify its argument"). We call it only to learn the matched pattern
	// for span/metric tagging, then delegate to mux.ServeHTTP which re-matches
	// and correctly sets r.PathValue for wildcard segments like {slug}.
	_, pattern := m.mux.Handler(r)
	if pattern != "" {
		span := trace.SpanFromContext(r.Context())
		span.SetName(r.Method + " " + pattern)
		if labeler, ok := otelhttp.LabelerFromContext(r.Context()); ok {
			labeler.Add(attribute.String("http.route", pattern))
		}
		// Fill the route container injected by HTTPMetricsMiddleware so
		// its defer can read the matched pattern after ServeHTTP returns.
		if rc, ok := r.Context().Value(routeKey{}).(*routeContainer); ok {
			rc.pattern = pattern
		}
	}
	m.mux.ServeHTTP(w, r)
}

// ── TraceMiddleware ──────────────────────────────────────────────────────────

// TraceMiddleware wraps next with otelhttp, starting an OTel span for every
// request and propagating W3C traceparent headers. It must be the outermost
// middleware so the span is active for all downstream code.
func TraceMiddleware(next http.Handler) http.Handler {
	return otelhttp.NewHandler(next, "")
}

// ── PanicMiddleware ──────────────────────────────────────────────────────────

// PanicMiddleware recovers from panics in downstream handlers, logs the stack
// trace, marks the active span as an error, and returns 500.
func PanicMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rw := &responseWriter{ResponseWriter: w}
		defer func() { //nolint:contextcheck
			v := recover()
			if v == nil {
				return
			}

			stack := debug.Stack()
			var err error
			switch e := v.(type) {
			case error:
				err = e
			default:
				err = fmt.Errorf("%v", e) //nolint:err113
			}

			Logger(r.Context()).ErrorContext(r.Context(), "panic recovered",
				"error", err,
				"stack", string(stack),
			)

			span := trace.SpanFromContext(r.Context())
			span.SetStatus(codes.Error, "panic")
			span.RecordError(err)

			if !rw.written {
				httputil.InternalServerError(w, r)
			}
		}()
		next.ServeHTTP(rw, r)
	})
}

// ── RequestMiddleware ────────────────────────────────────────────────────────

// RequestMiddleware assigns a request ID (trace ID when a span is active,
// UUIDv7 otherwise), sets X-Request-ID on the response, and logs a canonical
// request line at completion. It must run inside TraceMiddleware (span active)
// and after the auth middleware (user in context).
func RequestMiddleware(skipPaths ...string) func(http.Handler) http.Handler { //nolint:gocognit
	skip := make(map[string]bool, len(skipPaths))
	for _, p := range skipPaths {
		skip[p] = true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Derive request ID from active trace span or generate a UUID v7.
			var rid string
			span := trace.SpanFromContext(r.Context())
			if sc := span.SpanContext(); sc.IsValid() {
				rid = sc.TraceID().String()
			} else {
				rid = uuid.Must(uuid.NewV7()).String()
			}

			// Store in context and propagate to client.
			ctx := context.WithValue(r.Context(), requestIDKey{}, rid)
			r = r.WithContext(ctx)
			w.Header().Set("X-Request-ID", rid)

			// Capture status code.
			rw := &responseWriter{ResponseWriter: w}

			defer func() {
				status := rw.status()
				if skip[r.URL.Path] && status >= 200 && status < 300 {
					return
				}

				sc := span.SpanContext()

				attrs := []any{
					"method", r.Method,
					"path", r.URL.Path, // never r.URL.RequestURI() — no query strings
					"status", status,
					"duration_ms", float64(time.Since(start).Microseconds()) / 1000.0,
					"request_id", rid,
				}
				if sc.IsValid() {
					attrs = append(attrs,
						"trace_id", sc.TraceID().String(),
						"span_id", sc.SpanID().String(),
					)
				}
				if u, ok := middleware.FromContext(ctx); ok {
					attrs = append(attrs, "user.id", u.ID)
				}

				slog.Default().InfoContext(ctx, "request", attrs...)
			}()

			next.ServeHTTP(rw, r)
		})
	}
}

// ── HTTPMetricsMiddleware ────────────────────────────────────────────────────

// HTTPMetricsMiddleware records http_requests_total and
// http_request_duration_seconds for every request. It must run outside
// PanicMiddleware so that panics are recorded with status 500: PanicMiddleware
// writes its 500 response through the responseWriter captured here.
//
// It injects a *routeContainer into context; InstrumentedMux fills it with
// the matched route pattern so the defer can use it as a label value.
func HTTPMetricsMiddleware(m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx, rc := withRouteContainer(r.Context())
			r = r.WithContext(ctx)
			rw := &responseWriter{ResponseWriter: w}
			start := time.Now()

			defer func() {
				dur := time.Since(start).Seconds()
				route := rc.pattern
				if route == "" {
					route = "unknown"
				}
				status := strconv.Itoa(rw.status())
				m.HTTPRequests.WithLabelValues(r.Method, route, status).Inc()
				m.HTTPDuration.WithLabelValues(r.Method, route, status).Observe(dur)
			}()

			next.ServeHTTP(rw, r)
		})
	}
}

// ── Logger ───────────────────────────────────────────────────────────────────

// Logger returns the default slog logger pre-decorated with trace_id, span_id,
// request_id, and user.id extracted from ctx. Call this in handlers and
// services instead of bare slog.Info so every log line carries request context.
func Logger(ctx context.Context) *slog.Logger {
	attrs := make([]any, 0, 8)

	span := trace.SpanFromContext(ctx)
	if sc := span.SpanContext(); sc.IsValid() {
		attrs = append(attrs,
			slog.String("trace_id", sc.TraceID().String()),
			slog.String("span_id", sc.SpanID().String()),
		)
	}

	if rid, ok := RequestIDFromContext(ctx); ok {
		attrs = append(attrs, slog.String("request_id", rid))
	}

	if u, ok := middleware.FromContext(ctx); ok && u.ID != "" {
		attrs = append(attrs, slog.String("user.id", u.ID))
	}

	return slog.Default().With(attrs...)
}

// ── responseWriter ───────────────────────────────────────────────────────────

// responseWriter wraps http.ResponseWriter to capture the HTTP status code and
// whether WriteHeader has been called (used by panic recovery to avoid double
// writes).
type responseWriter struct {
	http.ResponseWriter
	code    int
	written bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if !rw.written {
		rw.code = code
		rw.written = true
		rw.ResponseWriter.WriteHeader(code)
	}
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.written {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b) //nolint:wrapcheck
}

func (rw *responseWriter) status() int {
	if rw.code == 0 {
		return http.StatusOK
	}
	return rw.code
}

// Unwrap allows http.ResponseController and other wrappers to reach the
// underlying ResponseWriter (e.g. for Flush, Hijack, Push).
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}
