package obs

import (
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// testProxyCIDRs returns the trusted proxy CIDR used throughout middleware tests.
// The connecting RemoteAddr in httptest requests defaults to 192.0.2.1.
func testProxyCIDRs(t *testing.T) []*net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR("192.0.2.0/24")
	if err != nil {
		t.Fatalf("bad test CIDR: %v", err)
	}
	return []*net.IPNet{n}
}

// ── extractIP (always RemoteAddr) ────────────────────────────────────────────

func TestExtractIP_IgnoresXFF(t *testing.T) {
	// extractIP must ignore X-Forwarded-For regardless of content.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("X-Forwarded-For", "10.0.0.1")
	if got := extractIP(r); got != "192.0.2.1" {
		t.Errorf("got %q, want %q (XFF must be ignored)", got, "192.0.2.1")
	}
}

// ── extractIPWithTrust (XFF trusted only from trusted proxy) ─────────────────

func TestExtractIP_XFF_Single(t *testing.T) {
	// Connecting IP (192.0.2.1) is in the trusted CIDR → XFF is trusted.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("X-Forwarded-For", "10.0.0.1")
	if got := extractIPWithTrust(r, testProxyCIDRs(t)); got != "10.0.0.1" {
		t.Errorf("got %q, want %q", got, "10.0.0.1")
	}
}

func TestExtractIP_XFF_Multiple(t *testing.T) {
	// Only the first (client) IP should be returned.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("X-Forwarded-For", "10.0.0.1, 172.16.0.1, 192.168.1.1")
	if got := extractIPWithTrust(r, testProxyCIDRs(t)); got != "10.0.0.1" {
		t.Errorf("got %q, want %q", got, "10.0.0.1")
	}
}

func TestExtractIP_XFF_WithSpaces(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("X-Forwarded-For", "  203.0.113.5  ,  10.0.0.2")
	if got := extractIPWithTrust(r, testProxyCIDRs(t)); got != "203.0.113.5" {
		t.Errorf("got %q, want %q", got, "203.0.113.5")
	}
}

func TestExtractIP_XFF_UntrustedProxy(t *testing.T) {
	// Connecting IP is NOT in trusted CIDR → must fall back to RemoteAddr.
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "1.2.3.4:1234"
	r.Header.Set("X-Forwarded-For", "10.0.0.1")
	if got := extractIPWithTrust(r, testProxyCIDRs(t)); got != "1.2.3.4" {
		t.Errorf("got %q, want %q (untrusted proxy XFF must be ignored)", got, "1.2.3.4")
	}
}

func TestExtractIP_RemoteAddr_Fallback(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	// httptest.NewRequest sets RemoteAddr to "192.0.2.1:1234"
	r.RemoteAddr = "192.0.2.1:1234"
	if got := extractIP(r); got != "192.0.2.1" {
		t.Errorf("got %q, want %q", got, "192.0.2.1")
	}
}

func TestExtractIP_IPv6_RemoteAddr(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "[::1]:8080"
	if got := extractIP(r); got != "::1" {
		t.Errorf("got %q, want %q", got, "::1")
	}
}

// ── SourceIPMiddleware ───────────────────────────────────────────────────────

func TestSourceIPMiddleware_InjectsRemoteAddr(t *testing.T) {
	// SourceIPMiddleware never trusts XFF; the injected IP must be RemoteAddr.
	var gotIP, gotUA string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIP = SourceIPFromContext(r.Context())
		gotUA = UserAgentFromContext(r.Context())
	})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("X-Forwarded-For", "10.1.2.3") // must be ignored
	r.Header.Set("User-Agent", "TestBot/1.0")
	w := httptest.NewRecorder()

	SourceIPMiddleware(inner).ServeHTTP(w, r)

	if gotIP != "192.0.2.1" {
		t.Errorf("source IP: got %q, want %q", gotIP, "192.0.2.1")
	}
	if gotUA != "TestBot/1.0" {
		t.Errorf("user agent: got %q, want %q", gotUA, "TestBot/1.0")
	}
}

func TestSourceIPMiddlewareWithTrustedCIDRs_InjectsXFF(t *testing.T) {
	// When the connecting IP is in the trusted CIDR, XFF must be trusted.
	var gotIP string
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotIP = SourceIPFromContext(r.Context())
	})

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "192.0.2.1:1234"
	r.Header.Set("X-Forwarded-For", "10.1.2.3")
	w := httptest.NewRecorder()

	SourceIPMiddlewareWithTrustedCIDRs(testProxyCIDRs(t))(inner).ServeHTTP(w, r)

	if gotIP != "10.1.2.3" {
		t.Errorf("source IP: got %q, want %q", gotIP, "10.1.2.3")
	}
}

func TestSourceIPFromContext_Empty(t *testing.T) {
	// Without middleware the context has no IP; should return empty string, not panic.
	ctx := httptest.NewRequest(http.MethodGet, "/", nil).Context()
	if got := SourceIPFromContext(ctx); got != "" {
		t.Errorf("expected empty string for missing IP, got %q", got)
	}
}

// ── extractQueryName ─────────────────────────────────────────────────────────

func TestExtractQueryName_NameComment(t *testing.T) {
	sql := "-- name: GetUserByID :one\nSELECT id FROM users WHERE id = $1"
	if got := extractQueryName(sql); got != "GetUserByID" {
		t.Errorf("got %q, want %q", got, "GetUserByID")
	}
}

func TestExtractQueryName_ExecComment(t *testing.T) {
	sql := "-- name: DeleteSession :exec\nDELETE FROM sessions WHERE token = $1"
	if got := extractQueryName(sql); got != "DeleteSession" {
		t.Errorf("got %q, want %q", got, "DeleteSession")
	}
}

func TestExtractQueryName_FallbackFirstWord(t *testing.T) {
	sql := "SELECT id FROM users"
	if got := extractQueryName(sql); got != "SELECT" {
		t.Errorf("fallback: got %q, want %q", got, "SELECT")
	}
}

func TestExtractQueryName_Empty(t *testing.T) {
	// Should not panic; returns "" for empty input.
	got := extractQueryName("")
	if got != "" {
		t.Errorf("empty SQL: got %q, want empty", got)
	}
}

// ── PanicMiddleware ──────────────────────────────────────────────────────────

func silenceLogger(t *testing.T) {
	t.Helper()
	old := slog.Default()
	slog.SetDefault(slog.New(slog.DiscardHandler))
	t.Cleanup(func() { slog.SetDefault(old) })
}

func TestPanicMiddleware_Returns500(t *testing.T) {
	silenceLogger(t)
	handler := PanicMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestPanicMiddleware_NoDoubleWrite(t *testing.T) {
	silenceLogger(t)
	// Handler writes 200 then panics; PanicMiddleware must not attempt a second write.
	handler := PanicMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		panic("late panic")
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	// Status was already 200; PanicMiddleware should not overwrite it.
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (already written), got %d", w.Code)
	}
}

func TestPanicMiddleware_NoPanic_Passthrough(t *testing.T) {
	handler := PanicMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
}

// ── RequestMiddleware ────────────────────────────────────────────────────────

func TestRequestMiddleware_SetsRequestIDHeader(t *testing.T) {
	silenceLogger(t)
	handler := RequestMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	rid := w.Header().Get("X-Request-ID")
	if rid == "" {
		t.Error("expected X-Request-ID header to be set")
	}
}

func TestRequestMiddleware_RequestIDInContext(t *testing.T) {
	silenceLogger(t)
	var gotRID string
	handler := RequestMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid, ok := RequestIDFromContext(r.Context())
		if !ok {
			t.Error("RequestIDFromContext returned false")
		}
		gotRID = rid
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if gotRID == "" {
		t.Error("request ID in context was empty")
	}
	if w.Header().Get("X-Request-ID") != gotRID {
		t.Errorf("X-Request-ID header %q != context value %q",
			w.Header().Get("X-Request-ID"), gotRID)
	}
}

// ── HTTPMetricsMiddleware ────────────────────────────────────────────────────

func newTestMetrics(t *testing.T) (*prometheus.Registry, *Metrics) {
	t.Helper()
	reg := prometheus.NewRegistry()
	return reg, NewMetrics(reg)
}

func TestRequestMiddleware_SkipPath_PassesThroughNormally(t *testing.T) {
	silenceLogger(t)
	// A skipped path with a 2xx status should still get a response — only logging is suppressed.
	var reached bool
	handler := RequestMiddleware("/healthz")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !reached {
		t.Error("next handler should be called even for a skip path")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHTTPMetricsMiddleware_CounterIncremented(t *testing.T) {
	_, m := newTestMetrics(t)
	handler := HTTPMetricsMiddleware(m)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	count := testutil.ToFloat64(m.HTTPRequests.WithLabelValues("GET", "unknown", "200"))
	if count != 1 {
		t.Errorf("expected counter == 1, got %f", count)
	}
}

func TestHTTPMetricsMiddleware_DurationObserved(t *testing.T) {
	reg, m := newTestMetrics(t)
	handler := HTTPMetricsMiddleware(m)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))

	r := httptest.NewRequest(http.MethodPost, "/items", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	// Gather all metrics and find at least one duration observation.
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	var found bool
	for _, mf := range families {
		if mf.GetName() == "http_request_duration_seconds" {
			for _, metric := range mf.GetMetric() {
				if metric.GetHistogram().GetSampleCount() > 0 {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("expected at least one http_request_duration_seconds observation")
	}
}

func TestHTTPMetricsMiddleware_Status500_Recorded(t *testing.T) {
	silenceLogger(t)
	_, m := newTestMetrics(t)

	// PanicMiddleware runs inside HTTPMetricsMiddleware; panic → 500 must be captured.
	handler := HTTPMetricsMiddleware(m)(
		PanicMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			panic("boom")
		})),
	)

	r := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	count := testutil.ToFloat64(m.HTTPRequests.WithLabelValues("GET", "unknown", "500"))
	if count != 1 {
		t.Errorf("panic should record status=500, counter=%f", count)
	}
}
