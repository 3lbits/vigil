package compliance

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/testutil"
)

// ── test doubles ──────────────────────────────────────────────────────────────

type complianceQ struct {
	testutil.StubQuerier
	frameworks   []db.Framework
	fwListErr    error
	createFwErr  error
	getFw        db.Framework
	getFwErr     error
	updateFwErr  error
	deleteFwErr  error
	createReqErr error
	getReq       db.Requirement
	getReqErr    error
	updateReqErr error
	deleteReqErr error
}

func (q *complianceQ) ListFrameworks(_ context.Context) ([]db.Framework, error) {
	return q.frameworks, q.fwListErr
}
func (q *complianceQ) CountRequirementsByFramework(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, nil
}
func (q *complianceQ) CountCoveredRequirementsByFramework(_ context.Context, _ uuid.UUID) (int64, error) {
	return 0, nil
}
func (q *complianceQ) ListRequirementsByFramework(_ context.Context, _ uuid.UUID) ([]db.Requirement, error) {
	return nil, nil
}
func (q *complianceQ) ListMeasuresForRequirement(_ context.Context, _ uuid.UUID) ([]db.ListMeasuresForRequirementRow, error) {
	return nil, nil
}
func (q *complianceQ) CreateFramework(_ context.Context, _ db.CreateFrameworkParams) (db.Framework, error) {
	return db.Framework{}, q.createFwErr
}
func (q *complianceQ) GetFramework(_ context.Context, _ uuid.UUID) (db.Framework, error) {
	return q.getFw, q.getFwErr
}
func (q *complianceQ) UpdateFramework(_ context.Context, _ db.UpdateFrameworkParams) (db.Framework, error) {
	return db.Framework{}, q.updateFwErr
}
func (q *complianceQ) DeleteFramework(_ context.Context, _ uuid.UUID) error {
	return q.deleteFwErr
}
func (q *complianceQ) CreateRequirement(_ context.Context, _ db.CreateRequirementParams) (db.Requirement, error) {
	return db.Requirement{}, q.createReqErr
}
func (q *complianceQ) GetRequirement(_ context.Context, _ uuid.UUID) (db.Requirement, error) {
	return q.getReq, q.getReqErr
}
func (q *complianceQ) UpdateRequirement(_ context.Context, _ db.UpdateRequirementParams) (db.Requirement, error) {
	return db.Requirement{}, q.updateReqErr
}
func (q *complianceQ) DeleteRequirement(_ context.Context, _ uuid.UUID) error {
	return q.deleteReqErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func newEngine(t *testing.T) *authz.Engine {
	t.Helper()
	e, err := authz.New(context.Background(), `
		package authz
		import rego.v1
		default allow := false
		allow if { input.user.role == "admin" }
		allow if { input.user.role == "editor"; input.action == "update_own" }
	`)
	if err != nil {
		t.Fatalf("compile authz engine: %v", err)
	}
	return e
}

func adminCtx(r *http.Request) *http.Request {
	ctx := middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "00000000-0000-0000-0000-000000000001", Name: "Alice", Role: "admin",
	})
	return r.WithContext(ctx)
}

// ── List ──────────────────────────────────────────────────────────────────────

func TestList_OK(t *testing.T) {
	q := &complianceQ{frameworks: []db.Framework{{ID: uuid.New(), Name: "ISO 27001"}}}
	h := NewHandler(q, nil)

	r := httptest.NewRequest(http.MethodGet, "/compliance", nil)
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestList_DBError(t *testing.T) {
	h := NewHandler(&complianceQ{fwListErr: errors.New("db down")}, nil)

	r := httptest.NewRequest(http.MethodGet, "/compliance", nil)
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.List(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── CreateFramework ───────────────────────────────────────────────────────────

func TestCreateFramework_OK(t *testing.T) {
	h := NewHandler(&complianceQ{}, nil)

	form := url.Values{
		"name": {"ISO 27001"}, "short_name": {"ISO"}, "version": {"2022"}, "description": {"Info sec"},
	}
	r := httptest.NewRequest(http.MethodPost, "/compliance/frameworks", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.CreateFramework(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "success") {
		t.Errorf("expected success flash in redirect")
	}
}

func TestCreateFramework_DBError(t *testing.T) {
	h := NewHandler(&complianceQ{createFwErr: errors.New("db down")}, nil)

	form := url.Values{"name": {"ISO"}, "short_name": {"I"}}
	r := httptest.NewRequest(http.MethodPost, "/compliance/frameworks", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.CreateFramework(w, r)

	// Re-renders form (200, not redirect).
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (form re-render), got %d", w.Code)
	}
}

// ── EditFramework ─────────────────────────────────────────────────────────────

func TestEditFramework_OK(t *testing.T) {
	id := uuid.New()
	q := &complianceQ{getFw: db.Framework{ID: id, Name: "NIST"}}
	h := NewHandler(q, nil)

	r := httptest.NewRequest(http.MethodGet, "/compliance/frameworks/"+id.String()+"/edit", nil)
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.EditFramework(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestEditFramework_NotFound(t *testing.T) {
	id := uuid.New()
	h := NewHandler(&complianceQ{getFwErr: errors.New("not found")}, nil)

	r := httptest.NewRequest(http.MethodGet, "/compliance/frameworks/"+id.String()+"/edit", nil)
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.EditFramework(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── UpdateFramework ───────────────────────────────────────────────────────────

func TestUpdateFramework_OK(t *testing.T) {
	id := uuid.New()
	h := NewHandler(&complianceQ{}, newEngine(t))

	form := url.Values{"name": {"NIST 2.0"}, "short_name": {"NIST"}, "version": {"2"}, "description": {"Updated"}}
	r := httptest.NewRequest(http.MethodPost, "/compliance/frameworks/"+id.String(), strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.UpdateFramework(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "success") {
		t.Errorf("expected success flash")
	}
}

// ── DeleteFramework ───────────────────────────────────────────────────────────

func TestDeleteFramework_OK(t *testing.T) {
	id := uuid.New()
	h := NewHandler(&complianceQ{}, nil)

	r := httptest.NewRequest(http.MethodPost, "/compliance/frameworks/"+id.String()+"/delete", nil)
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.DeleteFramework(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "success") {
		t.Errorf("expected success flash")
	}
}

func TestDeleteFramework_DBError(t *testing.T) {
	id := uuid.New()
	h := NewHandler(&complianceQ{deleteFwErr: errors.New("constraint violation")}, nil)

	r := httptest.NewRequest(http.MethodPost, "/compliance/frameworks/"+id.String()+"/delete", nil)
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.DeleteFramework(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "error") {
		t.Errorf("expected error flash")
	}
}

// ── CreateRequirement ─────────────────────────────────────────────────────────

func TestCreateRequirement_OK(t *testing.T) {
	fwID := uuid.New()
	h := NewHandler(&complianceQ{}, nil)

	form := url.Values{
		"ref": {"A.1"}, "title": {"Access control"}, "description": {"Must restrict access"}, "sort_order": {"10"},
	}
	r := httptest.NewRequest(http.MethodPost, "/compliance/frameworks/"+fwID.String()+"/requirements", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", fwID.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.CreateRequirement(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "success") {
		t.Errorf("expected success flash")
	}
}

func TestCreateRequirement_DBError(t *testing.T) {
	fwID := uuid.New()
	h := NewHandler(&complianceQ{createReqErr: errors.New("db down")}, nil)

	form := url.Values{"ref": {"A.1"}, "title": {"Title"}}
	r := httptest.NewRequest(http.MethodPost, "/compliance/frameworks/"+fwID.String()+"/requirements", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", fwID.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.CreateRequirement(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (form re-render), got %d", w.Code)
	}
}

func TestCreateRequirement_InvalidFrameworkID(t *testing.T) {
	h := NewHandler(&complianceQ{}, nil)

	form := url.Values{"ref": {"A.1"}, "title": {"Title"}}
	r := httptest.NewRequest(http.MethodPost, "/compliance/frameworks/bad-id/requirements", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", "bad-id")
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.CreateRequirement(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDeleteRequirement_OK(t *testing.T) {
	id := uuid.New()
	fwID := uuid.New()
	h := NewHandler(&complianceQ{getReq: db.Requirement{ID: id, FrameworkID: fwID}}, nil)

	r := httptest.NewRequest(http.MethodPost, "/compliance/requirements/"+id.String()+"/delete", nil)
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.DeleteRequirement(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "success") {
		t.Errorf("expected success flash")
	}
}

func TestDeleteRequirement_DBError(t *testing.T) {
	id := uuid.New()
	fwID := uuid.New()
	h := NewHandler(&complianceQ{
		getReq:       db.Requirement{ID: id, FrameworkID: fwID},
		deleteReqErr: errors.New("constraint violation"),
	}, nil)

	r := httptest.NewRequest(http.MethodPost, "/compliance/requirements/"+id.String()+"/delete", nil)
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.DeleteRequirement(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "error") {
		t.Errorf("expected error flash")
	}
}
