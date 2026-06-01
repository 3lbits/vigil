package authz

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// newTestEngine loads the real authz.rego policy from cmd/server/policies.
// Tests always run against the deployed policy, preventing silent drift.
func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	policyPath := filepath.Join(filepath.Dir(file), "..", "..", "cmd", "server", "policies", "authz.rego")
	src, err := os.ReadFile(policyPath)
	if err != nil {
		t.Fatalf("read authz.rego: %v (path: %s)", err, policyPath)
	}
	e, err := New(context.Background(), string(src))
	if err != nil {
		t.Fatalf("compile policy: %v", err)
	}
	return e
}

func allow(t *testing.T, e *Engine, role, resource, action string) bool {
	t.Helper()
	ok, err := e.Allow(context.Background(), "uid-123", role, resource, action)
	if err != nil {
		t.Fatalf("Allow(%s,%s,%s): %v", role, resource, action, err)
	}
	return ok
}

// ── Admin ─────────────────────────────────────────────────────────────────────

func TestPolicy_AdminAllowsAll(t *testing.T) {
	e := newTestEngine(t)
	resources := []string{"frameworks", "requirements", "measures", "dashboard", "activities", "risk", "about", "me"}
	actions := []string{"read", "write", "delete"}

	for _, res := range resources {
		for _, act := range actions {
			if !allow(t, e, "admin", res, act) {
				t.Errorf("admin should be allowed: resource=%s action=%s", res, act)
			}
		}
	}
}

func TestPolicy_AdminAllowsNonDataResources(t *testing.T) {
	e := newTestEngine(t)
	if !allow(t, e, "admin", "users", "write") {
		t.Error("admin should be allowed for users/write")
	}
}

// ── Editor ────────────────────────────────────────────────────────────────────

func TestPolicy_EditorAllowsDataActions(t *testing.T) {
	e := newTestEngine(t)
	resources := []string{"frameworks", "requirements", "measures", "dashboard", "activities", "risk", "about", "me"}
	actions := []string{"read", "write", "delete"}

	for _, res := range resources {
		for _, act := range actions {
			if !allow(t, e, "editor", res, act) {
				t.Errorf("editor should be allowed: resource=%s action=%s", res, act)
			}
		}
	}
}

func TestPolicy_EditorDeniedForUsersResource(t *testing.T) {
	e := newTestEngine(t)
	for _, act := range []string{"read", "write", "delete"} {
		if allow(t, e, "editor", "users", act) {
			t.Errorf("editor should not be allowed for users/%s", act)
		}
	}
}

// ── Viewer ────────────────────────────────────────────────────────────────────

func TestPolicy_ViewerAllowsRead(t *testing.T) {
	e := newTestEngine(t)
	resources := []string{"frameworks", "requirements", "measures", "dashboard", "activities", "risk", "about", "me"}

	for _, res := range resources {
		if !allow(t, e, "viewer", res, "read") {
			t.Errorf("viewer should be allowed to read resource=%s", res)
		}
	}
}

func TestPolicy_ViewerDeniedWrite(t *testing.T) {
	e := newTestEngine(t)
	resources := []string{"frameworks", "requirements", "measures", "dashboard", "activities", "risk", "about", "me"}

	for _, res := range resources {
		if allow(t, e, "viewer", res, "write") {
			t.Errorf("viewer should not be allowed to write resource=%s", res)
		}
		if allow(t, e, "viewer", res, "delete") {
			t.Errorf("viewer should not be allowed to delete resource=%s", res)
		}
	}
}

func TestPolicy_ViewerDeniedUsersResource(t *testing.T) {
	e := newTestEngine(t)
	if allow(t, e, "viewer", "users", "read") {
		t.Error("viewer should not be allowed to read users")
	}
}

// ── Anonymous / unknown role ──────────────────────────────────────────────────

func TestPolicy_AnonymousDeniedAll(t *testing.T) {
	e := newTestEngine(t)
	resources := []string{"frameworks", "requirements", "measures", "dashboard", "activities", "risk", "about", "me", "users"}
	actions := []string{"read", "write", "delete"}

	for _, res := range resources {
		for _, act := range actions {
			if allow(t, e, "", res, act) {
				t.Errorf("anonymous should not be allowed: resource=%s action=%s", res, act)
			}
		}
	}
}

func TestPolicy_UnknownRoleDenied(t *testing.T) {
	e := newTestEngine(t)
	if allow(t, e, "superuser", "frameworks", "read") {
		t.Error("unknown role 'superuser' should be denied")
	}
}

// ── Contributor ───────────────────────────────────────────────────────────────

func allowWith(t *testing.T, e *Engine, role, resource, action string, extra map[string]any) bool {
	t.Helper()
	ok, err := e.Allow(context.Background(), "uid-123", role, resource, action, extra)
	if err != nil {
		t.Fatalf("Allow(%s,%s,%s): %v", role, resource, action, err)
	}
	return ok
}

func TestPolicy_ContributorCanReadAndWrite(t *testing.T) {
	e := newTestEngine(t)
	resources := []string{"frameworks", "requirements", "measures", "dashboard", "activities", "risk", "about", "me"}
	for _, res := range resources {
		if !allow(t, e, "contributor", res, "read") {
			t.Errorf("contributor should be allowed to read resource=%s", res)
		}
		if !allow(t, e, "contributor", res, "write") {
			t.Errorf("contributor should be allowed to write resource=%s", res)
		}
	}
}

func TestPolicy_ContributorCannotDelete(t *testing.T) {
	e := newTestEngine(t)
	resources := []string{"frameworks", "requirements", "measures", "activities", "risk"}
	for _, res := range resources {
		if allow(t, e, "contributor", res, "delete") {
			t.Errorf("contributor should not be allowed to delete resource=%s", res)
		}
	}
}

func TestPolicy_ContributorUpdateOwnAllowed(t *testing.T) {
	e := newTestEngine(t)
	resources := []string{"frameworks", "requirements", "measures", "activities", "risk"}
	for _, res := range resources {
		if !allowWith(t, e, "contributor", res, "update_own", map[string]any{"is_participant": true}) {
			t.Errorf("contributor should be allowed update_own when participant: resource=%s", res)
		}
	}
}

func TestPolicy_ContributorUpdateOthersDenied(t *testing.T) {
	e := newTestEngine(t)
	resources := []string{"frameworks", "requirements", "measures", "activities", "risk"}
	for _, res := range resources {
		if allowWith(t, e, "contributor", res, "update_own", map[string]any{"is_participant": false}) {
			t.Errorf("contributor should not be allowed update_own when not participant: resource=%s", res)
		}
	}
}

func TestPolicy_EditorUpdateOwnAlwaysAllowed(t *testing.T) {
	e := newTestEngine(t)
	resources := []string{"frameworks", "requirements", "measures", "activities", "risk"}
	for _, res := range resources {
		if !allowWith(t, e, "editor", res, "update_own", map[string]any{"is_participant": false}) {
			t.Errorf("editor should be allowed update_own regardless of participant: resource=%s", res)
		}
	}
}

func TestPolicy_RisksAcceptDecline(t *testing.T) {
	e := newTestEngine(t)
	for _, action := range []string{"accept", "decline"} {
		if !allowWith(t, e, "editor", "risks", action, map[string]any{"is_owner": true}) {
			t.Errorf("editor owner should be allowed: risks/%s", action)
		}
		if allowWith(t, e, "editor", "risks", action, map[string]any{"is_owner": false}) {
			t.Errorf("editor non-owner should be denied: risks/%s", action)
		}
		if !allowWith(t, e, "admin", "risks", action, map[string]any{"is_owner": true}) {
			t.Errorf("admin owner should be allowed: risks/%s", action)
		}
		if allowWith(t, e, "admin", "risks", action, map[string]any{"is_owner": false}) {
			t.Errorf("admin non-owner should be denied: risks/%s", action)
		}
		if allowWith(t, e, "viewer", "risks", action, map[string]any{"is_owner": true}) {
			t.Errorf("viewer should be denied: risks/%s", action)
		}
		if allowWith(t, e, "contributor", "risks", action, map[string]any{"is_owner": true}) {
			t.Errorf("contributor should be denied: risks/%s", action)
		}
	}
}

func TestPolicy_AdminReadWriteAdminOnly(t *testing.T) {
	e := newTestEngine(t)
	for _, action := range []string{"read", "write"} {
		if !allow(t, e, "admin", "admin", action) {
			t.Errorf("admin should be allowed: admin/%s", action)
		}
		if allow(t, e, "editor", "admin", action) {
			t.Errorf("editor should be denied: admin/%s", action)
		}
		if allow(t, e, "viewer", "admin", action) {
			t.Errorf("viewer should be denied: admin/%s", action)
		}
	}
}

func TestPolicy_UsersManageAdminOnly(t *testing.T) {
	e := newTestEngine(t)
	if !allow(t, e, "admin", "users", "manage") {
		t.Error("admin should be allowed for users/manage")
	}
	if allow(t, e, "editor", "users", "manage") {
		t.Error("editor should be denied for users/manage")
	}
}

func TestPolicy_AvvikRoleMatrix(t *testing.T) {
	e := newTestEngine(t)
	if !allow(t, e, "viewer", "avvik", "read") {
		t.Error("viewer should be allowed for avvik/read")
	}
	if allow(t, e, "viewer", "avvik", "write") {
		t.Error("viewer should be denied for avvik/write")
	}
	if !allow(t, e, "editor", "avvik", "write") {
		t.Error("editor should be allowed for avvik/write")
	}
	if !allow(t, e, "contributor", "avvik", "write") {
		t.Error("contributor should be allowed for avvik/write")
	}
}
