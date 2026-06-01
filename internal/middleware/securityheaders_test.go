package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func applySecurityHeaders(t *testing.T, env string) *httptest.ResponseRecorder {
	t.Helper()
	hstsEnabled := env == "production"
	var reached bool
	h := SecurityHeaders(hstsEnabled)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !reached {
		t.Error("next handler was not called")
	}
	return w
}

func TestSecurityHeaders_XContentTypeOptions(t *testing.T) {
	w := applySecurityHeaders(t, "")
	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Errorf("X-Content-Type-Options: got %q, want %q", got, "nosniff")
	}
}

func TestSecurityHeaders_FrameAncestors(t *testing.T) {
	w := applySecurityHeaders(t, "")
	csp := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Errorf("CSP missing frame-ancestors 'none', got %q", csp)
	}
	if got := w.Header().Get("X-Frame-Options"); got != "" {
		t.Errorf("X-Frame-Options should be absent (superseded by frame-ancestors), got %q", got)
	}
}

func TestSecurityHeaders_ReferrerPolicy(t *testing.T) {
	w := applySecurityHeaders(t, "")
	if got := w.Header().Get("Referrer-Policy"); got != "strict-origin-when-cross-origin" {
		t.Errorf("Referrer-Policy: got %q, want %q", got, "strict-origin-when-cross-origin")
	}
}

func TestSecurityHeaders_PermissionsPolicy(t *testing.T) {
	w := applySecurityHeaders(t, "")
	if got := w.Header().Get("Permissions-Policy"); got != "camera=(), microphone=(), geolocation=()" {
		t.Errorf("Permissions-Policy: got %q, want %q", got, "camera=(), microphone=(), geolocation=()")
	}
}

func TestSecurityHeaders_COOP(t *testing.T) {
	w := applySecurityHeaders(t, "")
	if got := w.Header().Get("Cross-Origin-Opener-Policy"); got != "same-origin" {
		t.Errorf("Cross-Origin-Opener-Policy: got %q, want %q", got, "same-origin")
	}
}

func TestSecurityHeaders_CORP(t *testing.T) {
	w := applySecurityHeaders(t, "")
	if got := w.Header().Get("Cross-Origin-Resource-Policy"); got != "same-origin" {
		t.Errorf("Cross-Origin-Resource-Policy: got %q, want %q", got, "same-origin")
	}
}

func TestSecurityHeaders_CacheControl(t *testing.T) {
	w := applySecurityHeaders(t, "")
	if got := w.Header().Get("Cache-Control"); got != "no-store, private" {
		t.Errorf("Cache-Control: got %q, want %q", got, "no-store, private")
	}
}

func TestSecurityHeaders_CSP_Present(t *testing.T) {
	w := applySecurityHeaders(t, "")
	csp := w.Header().Get("Content-Security-Policy")
	if csp == "" {
		t.Error("Content-Security-Policy header must be set")
	}
	for _, required := range []string{"default-src", "script-src", "style-src", "object-src 'none'", "form-action 'self'", "frame-ancestors 'none'", "base-uri 'self'"} {
		if !strings.Contains(csp, required) {
			t.Errorf("CSP missing directive %q", required)
		}
	}
	if strings.Contains(csp, "'unsafe-inline'") {
		t.Error("CSP must not include unsafe-inline")
	}
	if !strings.Contains(csp, "script-src 'self' 'nonce-") {
		t.Errorf("CSP script-src must include nonce, got %q", csp)
	}
}

func TestSecurityHeaders_HSTS_AbsentInDev(t *testing.T) {
	w := applySecurityHeaders(t, "") // APP_ENV not set
	if got := w.Header().Get("Strict-Transport-Security"); got != "" {
		t.Errorf("HSTS should not be set in non-production, got %q", got)
	}
}

func TestSecurityHeaders_HSTS_PresentInProduction(t *testing.T) {
	w := applySecurityHeaders(t, "production")
	hsts := w.Header().Get("Strict-Transport-Security")
	if hsts == "" {
		t.Error("HSTS must be set in production")
	}
	if !strings.Contains(hsts, "max-age=") {
		t.Errorf("HSTS must include max-age, got %q", hsts)
	}
	if !strings.Contains(hsts, "includeSubDomains") {
		t.Errorf("HSTS must include includeSubDomains, got %q", hsts)
	}
}

func TestSecurityHeaders_NextHandlerCalled(t *testing.T) {
	// Verify middleware does not short-circuit the chain.
	var code int
	h := SecurityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		code = http.StatusTeapot
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if code != http.StatusTeapot {
		t.Error("next handler was not called")
	}
}

func TestSecurityHeaders_SetsNonceInContext(t *testing.T) {
	var nonce string
	h := SecurityHeaders(false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nonce = CSPNonceFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if nonce == "" {
		t.Fatal("expected nonce in request context")
	}
	if !strings.Contains(w.Header().Get("Content-Security-Policy"), "nonce-"+nonce) {
		t.Fatal("expected response CSP header to contain request nonce")
	}
}
