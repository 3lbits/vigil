package authz

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3lbits/vigil/internal/middleware"
)

// newReachable returns a handler that sets a header so tests can verify it was called.
func newReachable() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Reached", "true")
		w.WriteHeader(http.StatusOK)
	})
}

// ── Unauthenticated ───────────────────────────────────────────────────────────

func TestRequirePolicy_UnauthenticatedRedirectsToLogin(t *testing.T) {
	e := newTestEngine(t)
	h := RequirePolicy(e, "frameworks", "read")(newReachable())

	r := httptest.NewRequest(http.MethodGet, "/compliance", nil)
	// No user in context.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302 redirect, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
	if w.Header().Get("X-Reached") == "true" {
		t.Error("handler should not be reached for unauthenticated request")
	}
}

// ── Viewer: read allowed, write/delete denied ─────────────────────────────────

func TestRequirePolicy_ViewerAllowedRead(t *testing.T) {
	e := newTestEngine(t)
	h := RequirePolicy(e, "frameworks", "read")(newReachable())

	r := httptest.NewRequest(http.MethodGet, "/compliance", nil)
	r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "uid-1", Role: "viewer",
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("viewer read: expected 200, got %d", w.Code)
	}
	if w.Header().Get("X-Reached") != "true" {
		t.Error("handler should be reached for allowed request")
	}
}

func TestRequirePolicy_ViewerDeniedWrite(t *testing.T) {
	e := newTestEngine(t)
	h := RequirePolicy(e, "frameworks", "write")(newReachable())

	r := httptest.NewRequest(http.MethodPost, "/compliance/frameworks", nil)
	r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "uid-1", Role: "viewer",
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("viewer write: expected 403, got %d", w.Code)
	}
	if w.Header().Get("X-Reached") == "true" {
		t.Error("handler should not be reached for denied request")
	}
}

func TestRequirePolicy_ViewerDeniedDelete(t *testing.T) {
	e := newTestEngine(t)
	h := RequirePolicy(e, "measures", "delete")(newReachable())

	r := httptest.NewRequest(http.MethodPost, "/measures/delete", nil)
	r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "uid-1", Role: "viewer",
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("viewer delete: expected 403, got %d", w.Code)
	}
}

// ── Editor ────────────────────────────────────────────────────────────────────

func TestRequirePolicy_EditorAllowedWriteActivities(t *testing.T) {
	e := newTestEngine(t)
	h := RequirePolicy(e, "activities", "write")(newReachable())

	r := httptest.NewRequest(http.MethodPost, "/activities", nil)
	r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "uid-2", Role: "editor",
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("editor write activities: expected 200, got %d", w.Code)
	}
}

func TestRequirePolicy_EditorDeniedUsers(t *testing.T) {
	e := newTestEngine(t)
	h := RequirePolicy(e, "users", "write")(newReachable())

	r := httptest.NewRequest(http.MethodPost, "/admin/users", nil)
	r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "uid-2", Role: "editor",
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("editor users/write: expected 403, got %d", w.Code)
	}
}

// ── Admin ─────────────────────────────────────────────────────────────────────

func TestRequirePolicy_AdminAllowedEverywhere(t *testing.T) {
	e := newTestEngine(t)
	resources := []string{"frameworks", "measures", "activities", "risk", "about", "users"}
	actions := []string{"read", "write", "delete"}

	for _, res := range resources {
		for _, act := range actions {
			h := RequirePolicy(e, res, act)(newReachable())
			r := httptest.NewRequest(http.MethodPost, "/", nil)
			r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
				ID: "uid-0", Role: "admin",
			}))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Errorf("admin %s/%s: expected 200, got %d", res, act, w.Code)
			}
		}
	}
}

// ── Next handler is called for allowed requests ───────────────────────────────

func TestRequirePolicy_AllowedRequestReachesHandler(t *testing.T) {
	e := newTestEngine(t)
	var reached bool
	h := RequirePolicy(e, "dashboard", "read")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
	}))

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "uid-3", Role: "editor",
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if !reached {
		t.Error("next handler should be called for allowed request")
	}
}

func TestRequirePolicy_RisksAccept_EditorDeniedWithoutObjectInput(t *testing.T) {
	e := newTestEngine(t)
	h := RequirePolicy(e, "risks", "accept")(newReachable())

	r := httptest.NewRequest(http.MethodPost, "/risk/assessments/id/accept", nil)
	r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "uid-7", Role: "editor",
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("editor risks/accept without object input: expected 403, got %d", w.Code)
	}
}

func TestRequirePolicy_RisksAccept_ViewerDenied(t *testing.T) {
	e := newTestEngine(t)
	h := RequirePolicy(e, "risks", "accept")(newReachable())

	r := httptest.NewRequest(http.MethodPost, "/risk/assessments/id/accept", nil)
	r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "uid-8", Role: "viewer",
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("viewer risks/accept: expected 403, got %d", w.Code)
	}
}

func TestRequirePolicy_AdminRead_AdminAllowedEditorDenied(t *testing.T) {
	e := newTestEngine(t)
	h := RequirePolicy(e, "admin", "read")(newReachable())

	adminReq := httptest.NewRequest(http.MethodGet, "/admin", nil)
	adminReq = adminReq.WithContext(middleware.SetUser(adminReq.Context(), middleware.SessionUser{
		ID: "uid-9", Role: "admin",
	}))
	adminW := httptest.NewRecorder()
	h.ServeHTTP(adminW, adminReq)
	if adminW.Code != http.StatusOK {
		t.Errorf("admin admin/read: expected 200, got %d", adminW.Code)
	}

	editorReq := httptest.NewRequest(http.MethodGet, "/admin", nil)
	editorReq = editorReq.WithContext(middleware.SetUser(editorReq.Context(), middleware.SessionUser{
		ID: "uid-10", Role: "editor",
	}))
	editorW := httptest.NewRecorder()
	h.ServeHTTP(editorW, editorReq)
	if editorW.Code != http.StatusForbidden {
		t.Errorf("editor admin/read: expected 403, got %d", editorW.Code)
	}
}

// ── RequireObjectPolicy ───────────────────────────────────────────────────────

func TestRequireObjectPolicy_UnauthenticatedRedirectsToLogin(t *testing.T) {
	e := newTestEngine(t)
	h := RequireObjectPolicy(e, "frameworks", "read", nil, nil)(newReachable())

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", w.Code)
	}
	if w.Header().Get("Location") != "/login" {
		t.Errorf("expected redirect to /login")
	}
}

func TestRequireObjectPolicy_LoaderReturnsFalse_HandlerNotCalled(t *testing.T) {
	e := newTestEngine(t)
	loader := func(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
		http.NotFound(w, r)
		return nil, false
	}
	h := RequireObjectPolicy(e, "frameworks", "read", loader, nil)(newReachable())

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "uid-1", Role: "admin",
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 from loader, got %d", w.Code)
	}
	if w.Header().Get("X-Reached") == "true" {
		t.Error("handler must not be called when loader returns false")
	}
}

func TestRequireObjectPolicy_BuildInputError_Returns500(t *testing.T) {
	e := newTestEngine(t)
	buildInput := func(r *http.Request, u middleware.SessionUser) (map[string]any, error) {
		return nil, errors.New("database unavailable")
	}
	h := RequireObjectPolicy(e, "frameworks", "read", nil, buildInput)(newReachable())

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "uid-1", Role: "admin",
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 when buildInput errors, got %d", w.Code)
	}
	if w.Header().Get("X-Reached") == "true" {
		t.Error("handler must not be called when buildInput errors")
	}
}

func TestRequireObjectPolicy_PolicyDenied_Returns403(t *testing.T) {
	// Use a policy that always denies to exercise the 403 branch in authorizeOrRespond.
	e, err := New(context.Background(), `
		package authz
		import rego.v1
		default allow := false
	`)
	if err != nil {
		t.Fatalf("compile policy: %v", err)
	}
	h := RequireObjectPolicy(e, "anything", "write", nil, nil)(newReachable())

	r := httptest.NewRequest(http.MethodPost, "/", nil)
	r = r.WithContext(middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "uid-1", Role: "admin",
	}))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403 when policy denies, got %d", w.Code)
	}
	if w.Header().Get("X-Reached") == "true" {
		t.Error("handler must not be called when policy denies")
	}
}
