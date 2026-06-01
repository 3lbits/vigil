package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/health"
	"github.com/3lbits/vigil/internal/testutil"
)

// ── SetUser / FromContext ─────────────────────────────────────────────────────

func TestSetUser_FromContext_RoundTrip(t *testing.T) {
	want := SessionUser{ID: "abc", Name: "Alice", Role: "admin", Email: "alice@example.com"}
	ctx := SetUser(context.Background(), want)
	got, ok := FromContext(ctx)

	if !ok {
		t.Fatal("FromContext returned ok=false after SetUser")
	}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestFromContext_MissingUser(t *testing.T) {
	_, ok := FromContext(context.Background())
	if ok {
		t.Error("expected ok=false on empty context")
	}
}

// ── StubMiddleware ────────────────────────────────────────────────────────────

func TestStubMiddleware_InjectsAdminUser(t *testing.T) {
	var got SessionUser
	var devStub bool
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		u, ok := FromContext(r.Context())
		if !ok {
			t.Error("expected user in context")
			return
		}
		got = u
		devStub = DevStubAuthFromContext(r.Context())
	})

	handler := StubMiddleware(inner)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if got.Role != "admin" {
		t.Errorf("expected role=admin, got %q", got.Role)
	}
	if got.ID == "" {
		t.Error("expected non-empty ID")
	}
	if !devStub {
		t.Error("expected dev-stub auth marker in context")
	}
}

func TestStubMiddleware_UsesAllowedRoleFromCookie(t *testing.T) {
	var got SessionUser
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		u, _ := FromContext(r.Context())
		got = u
	})

	handler := StubMiddleware(inner)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: DevRoleCookieName, Value: "viewer"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if got.Role != "viewer" {
		t.Errorf("expected role=viewer, got %q", got.Role)
	}
}

func TestStubMiddleware_InvalidCookieRoleFallsBackToAdmin(t *testing.T) {
	var got SessionUser
	inner := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		u, _ := FromContext(r.Context())
		got = u
	})

	handler := StubMiddleware(inner)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.AddCookie(&http.Cookie{Name: DevRoleCookieName, Value: "superadmin"})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if got.Role != "admin" {
		t.Errorf("expected role=admin, got %q", got.Role)
	}
}

// ── RequireAuth ───────────────────────────────────────────────────────────────

func TestRequireAuth_RedirectsAnonymous(t *testing.T) {
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := RequireAuth(inner)
	r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	// No user in context.
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

func TestRequireAuth_AllowsAuthenticatedUser(t *testing.T) {
	reached := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})

	handler := RequireAuth(inner)
	r := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	r = r.WithContext(SetUser(r.Context(), SessionUser{ID: "u1", Role: "viewer"}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !reached {
		t.Error("inner handler should have been called for authenticated user")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireLoginExcept_RedirectsAnonymousPrivatePath(t *testing.T) {
	reached := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	handler := RequireLoginExcept(
		[]PublicRoute{{Method: http.MethodGet, Path: "/login"}},
		[]string{"/public/", "/auth/"},
	)(inner)

	r := httptest.NewRequest(http.MethodGet, "/risks", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if reached {
		t.Error("inner handler should not be reached for anonymous private path")
	}
	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

func TestRequireLoginExcept_AllowsAnonymousPublicRoutes(t *testing.T) {
	reached := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	handler := RequireLoginExcept(
		[]PublicRoute{
			{Method: http.MethodGet, Path: "/login"},
			{Method: http.MethodPost, Path: "/locale"},
			{Method: http.MethodGet, Path: "/healthz"},
		},
		[]string{"/public/", "/auth/"},
	)(inner)

	for _, tc := range []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/login"},
		{http.MethodPost, "/locale"},
		{http.MethodGet, "/healthz"},
		{http.MethodHead, "/healthz"},
		{http.MethodGet, "/public/css/output.css"},
		{http.MethodGet, "/auth/oidc"},
	} {
		reached = false
		r := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		if !reached {
			t.Fatalf("expected inner handler for %s %s", tc.method, tc.path)
		}
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s %s, got %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestRequireLoginExcept_AllowsAuthenticatedPrivatePath(t *testing.T) {
	reached := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	handler := RequireLoginExcept(nil, nil)(inner)

	r := httptest.NewRequest(http.MethodGet, "/admin", nil)
	r = r.WithContext(SetUser(r.Context(), SessionUser{ID: "u1", Role: "viewer"}))
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !reached {
		t.Error("inner handler should be reached for authenticated private path")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// ── Chain ─────────────────────────────────────────────────────────────────────

func TestChain_OrderIsOuterToInner(t *testing.T) {
	// Each middleware appends its letter to a shared slice.
	// We verify outer (A) runs before inner (B) on the request path.
	var order []string

	mwA := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "A")
			next.ServeHTTP(w, r)
		})
	}
	mwB := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "B")
			next.ServeHTTP(w, r)
		})
	}

	core := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		order = append(order, "core")
		w.WriteHeader(http.StatusOK)
	})

	// Chain(core, mwA, mwB) means mwA is outermost.
	handler := Chain(core, mwA, mwB)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if len(order) != 3 {
		t.Fatalf("expected 3 calls, got %d: %v", len(order), order)
	}
	if order[0] != "A" || order[1] != "B" || order[2] != "core" {
		t.Errorf("expected [A B core], got %v", order)
	}
}

func TestChain_NoMiddleware(t *testing.T) {
	reached := false
	core := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	handler := Chain(core)
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if !reached {
		t.Error("core handler should be reached with no middleware")
	}
}

type moduleFlagsStubQ struct {
	testutil.StubQuerier
	settings []moduleFlagsSettingsResult
	calls    int
}

type moduleFlagsSettingsResult struct {
	settings db.AppSetting
	err      error
}

func (q *moduleFlagsStubQ) GetAppSettings(_ context.Context) (db.AppSetting, error) {
	if len(q.settings) == 0 {
		return db.AppSetting{}, errors.New("no settings configured")
	}
	idx := q.calls
	if idx >= len(q.settings) {
		idx = len(q.settings) - 1
	}
	q.calls++
	return q.settings[idx].settings, q.settings[idx].err
}

func TestModuleFlagsMiddleware_SuccessAppliesSettings(t *testing.T) {
	health.SetModuleFlagsSettingsLoadFailure(false)
	t.Cleanup(func() { health.SetModuleFlagsSettingsLoadFailure(false) })

	q := &moduleFlagsStubQ{
		settings: []moduleFlagsSettingsResult{
			{settings: db.AppSetting{ComplianceEnabled: false, RiskEnabled: true, ActivitiesEnabled: true, AssetsEnabled: true}},
		},
	}

	inner := RequireModule("compliance")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler := ModuleFlagsMiddleware(q, true)(inner)

	r := httptest.NewRequest(http.MethodGet, "/compliance", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for disabled compliance module, got %d", w.Code)
	}
}

func TestModuleFlagsMiddleware_ErrorUsesLastKnownGood(t *testing.T) {
	health.SetModuleFlagsSettingsLoadFailure(false)
	t.Cleanup(func() { health.SetModuleFlagsSettingsLoadFailure(false) })

	q := &moduleFlagsStubQ{
		settings: []moduleFlagsSettingsResult{
			{settings: db.AppSetting{ComplianceEnabled: false, RiskEnabled: true, ActivitiesEnabled: true, AssetsEnabled: true}},
			{err: errors.New("db read failed")},
		},
	}

	inner := RequireModule("compliance")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler := ModuleFlagsMiddleware(q, true)(inner)

	r1 := httptest.NewRequest(http.MethodGet, "/compliance", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)
	if w1.Code != http.StatusNotFound {
		t.Fatalf("expected first request 404 from disabled compliance module, got %d", w1.Code)
	}

	r2 := httptest.NewRequest(http.MethodGet, "/compliance", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	if w2.Code != http.StatusNotFound {
		t.Fatalf("expected cached disabled compliance module to stay 404 on settings error, got %d", w2.Code)
	}
}

func TestModuleFlagsMiddleware_ColdCacheErrorUsesDocumentedDefault(t *testing.T) {
	health.SetModuleFlagsSettingsLoadFailure(false)
	t.Cleanup(func() { health.SetModuleFlagsSettingsLoadFailure(false) })

	q := &moduleFlagsStubQ{
		settings: []moduleFlagsSettingsResult{
			{err: errors.New("db down")},
		},
	}

	inner := RequireModule("risk")(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	handler := ModuleFlagsMiddleware(q, true)(inner)

	r := httptest.NewRequest(http.MethodGet, "/risks", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if moduleFlagsColdCacheFailClosed {
		if w.Code != http.StatusNotFound {
			t.Fatalf("expected fail-closed cold-cache default to return 404, got %d", w.Code)
		}
		return
	}
	if w.Code != http.StatusOK {
		t.Fatalf("expected fail-open cold-cache default to allow request, got %d", w.Code)
	}
}

func TestModuleFlagsMiddleware_HealthSignalSetAndCleared(t *testing.T) {
	health.SetModuleFlagsSettingsLoadFailure(false)
	t.Cleanup(func() { health.SetModuleFlagsSettingsLoadFailure(false) })

	q := &moduleFlagsStubQ{
		settings: []moduleFlagsSettingsResult{
			{err: errors.New("temporary settings error")},
			{settings: db.AppSetting{ComplianceEnabled: true, RiskEnabled: true, ActivitiesEnabled: true, AssetsEnabled: true}},
		},
	}

	handler := ModuleFlagsMiddleware(q, true)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	r1 := httptest.NewRequest(http.MethodGet, "/", nil)
	w1 := httptest.NewRecorder()
	handler.ServeHTTP(w1, r1)
	if !health.ModuleFlagsSettingsLoadFailed() {
		t.Fatal("expected health signal set after app_settings load failure")
	}

	r2 := httptest.NewRequest(http.MethodGet, "/", nil)
	w2 := httptest.NewRecorder()
	handler.ServeHTTP(w2, r2)
	if health.ModuleFlagsSettingsLoadFailed() {
		t.Fatal("expected health signal cleared after app_settings load recovery")
	}
}
