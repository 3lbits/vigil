package measures

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/testutil"
	"github.com/google/uuid"
)

// ── test doubles ──────────────────────────────────────────────────────────────

type measuresQ struct {
	testutil.StubQuerier
	measures             []db.Measure
	listErr              error
	createErr            error
	updateErr            error
	deleteErr            error
	getMeasure           db.Measure
	getErr               error
	linkedRisks          []db.Risk
	flaggedRiskIDs       []uuid.UUID
	reassessmentEventLog []db.CreateRiskReassessmentEventParams
	users                []db.User
	usersErr             error
}

func (q *measuresQ) ListMeasures(_ context.Context) ([]db.Measure, error) {
	return q.measures, q.listErr
}
func (q *measuresQ) FilterMeasures(_ context.Context, _ db.FilterMeasuresParams) ([]db.Measure, error) {
	return q.measures, q.listErr
}
func (q *measuresQ) ListMeasureFrameworkLinks(_ context.Context) ([]db.ListMeasureFrameworkLinksRow, error) {
	return nil, nil
}
func (q *measuresQ) CreateMeasure(_ context.Context, _ db.CreateMeasureParams) (db.Measure, error) {
	return db.Measure{}, q.createErr
}
func (q *measuresQ) GetMeasure(_ context.Context, _ uuid.UUID) (db.Measure, error) {
	return q.getMeasure, q.getErr
}
func (q *measuresQ) UpdateMeasure(_ context.Context, arg db.UpdateMeasureParams) (db.Measure, error) {
	return db.Measure{
		ID:     arg.ID,
		Status: arg.Status,
		Name:   arg.Name,
	}, q.updateErr
}
func (q *measuresQ) DeleteMeasure(_ context.Context, _ uuid.UUID) error {
	return q.deleteErr
}
func (q *measuresQ) ListRisksForMeasure(_ context.Context, _ uuid.UUID) ([]db.Risk, error) {
	return q.linkedRisks, nil
}
func (q *measuresQ) FlagRiskForReview(_ context.Context, id uuid.UUID) error {
	q.flaggedRiskIDs = append(q.flaggedRiskIDs, id)
	return nil
}
func (q *measuresQ) CreateRiskReassessmentEvent(_ context.Context, arg db.CreateRiskReassessmentEventParams) error {
	q.reassessmentEventLog = append(q.reassessmentEventLog, arg)
	return nil
}
func (q *measuresQ) ListUsers(_ context.Context) ([]db.User, error) {
	return q.users, q.usersErr
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

func TestList_FullPage(t *testing.T) {
	h := NewHandler(&measuresQ{measures: []db.Measure{{ID: uuid.New(), Name: "Firewall"}}}, nil)

	r := httptest.NewRequest(http.MethodGet, "/measures", nil)
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestList_HTMXPartial(t *testing.T) {
	h := NewHandler(&measuresQ{}, nil)

	r := httptest.NewRequest(http.MethodGet, "/measures", nil)
	r.Header.Set("HX-Request", "true")
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	// HTMX partial should not contain the full layout navigation.
	// The response should be shorter than a full page.
	body := w.Body.String()
	if strings.Contains(body, "<html") {
		t.Error("HTMX partial should not render full HTML document")
	}
}

func TestList_DBError(t *testing.T) {
	h := NewHandler(&measuresQ{listErr: errors.New("db down")}, nil)

	r := httptest.NewRequest(http.MethodGet, "/measures", nil)
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.List(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── Create ────────────────────────────────────────────────────────────────────

func TestCreate_OK(t *testing.T) {
	h := NewHandler(&measuresQ{}, nil)

	form := url.Values{
		"name":        {"Firewall rules"},
		"description": {"Block inbound"},
		"category":    {"network"},
		"owner":       {"ops"},
		"status":      {"implemented"},
	}
	r := httptest.NewRequest(http.MethodPost, "/measures", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Create(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "/measures") {
		t.Errorf("expected redirect to /measures, got %q", loc)
	}
}

func TestCreate_DBError(t *testing.T) {
	h := NewHandler(&measuresQ{createErr: errors.New("db down")}, nil)

	form := url.Values{"name": {"X"}, "status": {"planned"}}
	r := httptest.NewRequest(http.MethodPost, "/measures", strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Create(w, r)

	// Re-renders the form with an error message (not a redirect).
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (form re-render), got %d", w.Code)
	}
}

// ── Delete ────────────────────────────────────────────────────────────────────

func TestDelete_OK(t *testing.T) {
	id := uuid.New()
	h := NewHandler(&measuresQ{}, nil)

	r := httptest.NewRequest(http.MethodPost, "/measures/"+id.String()+"/delete", nil)
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Delete(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "success") {
		t.Errorf("expected success flash")
	}
}

func TestDelete_DBError(t *testing.T) {
	id := uuid.New()
	h := NewHandler(&measuresQ{deleteErr: errors.New("db down")}, nil)

	r := httptest.NewRequest(http.MethodPost, "/measures/"+id.String()+"/delete", nil)
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Delete(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "error") {
		t.Errorf("expected error flash")
	}
}

func TestDelete_InvalidID(t *testing.T) {
	h := NewHandler(&measuresQ{}, nil)

	r := httptest.NewRequest(http.MethodPost, "/measures/bad-id/delete", nil)
	r.SetPathValue("id", "bad-id")
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Delete(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── Edit ──────────────────────────────────────────────────────────────────────

func TestEdit_OK(t *testing.T) {
	id := uuid.New()
	h := NewHandler(&measuresQ{getMeasure: db.Measure{ID: id, Name: "Firewall"}}, nil)

	r := httptest.NewRequest(http.MethodGet, "/measures/"+id.String()+"/edit", nil)
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Edit(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestEdit_NotFound(t *testing.T) {
	id := uuid.New()
	h := NewHandler(&measuresQ{getErr: errors.New("not found")}, nil)

	r := httptest.NewRequest(http.MethodGet, "/measures/"+id.String()+"/edit", nil)
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Edit(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── Update ────────────────────────────────────────────────────────────────────

func TestUpdate_OK(t *testing.T) {
	id := uuid.New()
	h := NewHandler(&measuresQ{}, newEngine(t))

	form := url.Values{
		"name": {"Updated name"}, "status": {"implemented"},
	}
	r := httptest.NewRequest(http.MethodPost, "/measures/"+id.String(), strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Update(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
}

func TestShouldFlagRiskReassessment(t *testing.T) {
	tests := []struct {
		name string
		old  string
		new  string
		want bool
	}{
		{name: "implemented transition", old: "planned", new: "implemented", want: true},
		{name: "deprecated transition", old: "in_progress", new: "deprecated", want: true},
		{name: "no change implemented", old: "implemented", new: "implemented", want: false},
		{name: "non-trigger transition", old: "planned", new: "in_progress", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldFlagRiskReassessment(tc.old, tc.new)
			if got != tc.want {
				t.Fatalf("shouldFlagRiskReassessment(%q,%q)=%v want %v", tc.old, tc.new, got, tc.want)
			}
		})
	}
}

func TestUpdate_StatusImplementedFlagsLinkedRisks(t *testing.T) {
	id := uuid.New()
	r1 := uuid.New()
	r2 := uuid.New()
	q := &measuresQ{
		getMeasure:  db.Measure{ID: id, Status: "planned"},
		linkedRisks: []db.Risk{{ID: r1}, {ID: r2}},
	}
	h := NewHandler(q, newEngine(t))

	form := url.Values{
		"name":   {"Updated name"},
		"status": {"implemented"},
	}
	r := httptest.NewRequest(http.MethodPost, "/measures/"+id.String(), strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Update(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if len(q.flaggedRiskIDs) != 2 {
		t.Fatalf("expected 2 flagged risks, got %d", len(q.flaggedRiskIDs))
	}
	if len(q.reassessmentEventLog) != 2 {
		t.Fatalf("expected 2 reassessment events, got %d", len(q.reassessmentEventLog))
	}
	for _, ev := range q.reassessmentEventLog {
		if ev.TriggerStatus != "implemented" {
			t.Fatalf("expected trigger status implemented, got %q", ev.TriggerStatus)
		}
	}
}

func TestUpdate_StatusDeprecatedFlagsLinkedRisks(t *testing.T) {
	id := uuid.New()
	r1 := uuid.New()
	q := &measuresQ{
		getMeasure:  db.Measure{ID: id, Status: "implemented"},
		linkedRisks: []db.Risk{{ID: r1}},
	}
	h := NewHandler(q, newEngine(t))

	form := url.Values{
		"name":   {"Updated name"},
		"status": {"deprecated"},
	}
	r := httptest.NewRequest(http.MethodPost, "/measures/"+id.String(), strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Update(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if len(q.flaggedRiskIDs) != 1 {
		t.Fatalf("expected 1 flagged risk, got %d", len(q.flaggedRiskIDs))
	}
	if len(q.reassessmentEventLog) != 1 {
		t.Fatalf("expected 1 reassessment event, got %d", len(q.reassessmentEventLog))
	}
	if q.reassessmentEventLog[0].TriggerStatus != "deprecated" {
		t.Fatalf("expected trigger status deprecated, got %q", q.reassessmentEventLog[0].TriggerStatus)
	}
}

func TestUpdate_SameStatusDoesNotFlagRisks(t *testing.T) {
	id := uuid.New()
	q := &measuresQ{
		getMeasure:  db.Measure{ID: id, Status: "implemented"},
		linkedRisks: []db.Risk{{ID: uuid.New()}},
	}
	h := NewHandler(q, newEngine(t))

	form := url.Values{
		"name":   {"Updated name"},
		"status": {"implemented"},
	}
	r := httptest.NewRequest(http.MethodPost, "/measures/"+id.String(), strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", id.String())
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Update(w, r)

	if w.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d", w.Code)
	}
	if len(q.flaggedRiskIDs) != 0 {
		t.Fatalf("expected no flagged risks, got %d", len(q.flaggedRiskIDs))
	}
	if len(q.reassessmentEventLog) != 0 {
		t.Fatalf("expected no reassessment events, got %d", len(q.reassessmentEventLog))
	}
}

func TestParseMeasureAssignee(t *testing.T) {
	uid := uuid.New()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(url.Values{
		"owner":           {"Alice"},
		"assignee_id":     {uid.String()},
		"assignee_lookup": {"Bob"},
	}.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatalf("parse form: %v", err)
	}
	owner, assignee := parseMeasureAssignee(r, &measuresQ{})
	if owner != "Alice" || !assignee.Valid || assignee.UUID != uid {
		t.Fatalf("unexpected assignee resolution with assignee_id: owner=%q assignee=%v", owner, assignee)
	}

	q := &measuresQ{
		users: []db.User{
			{ID: uid, Name: "Bob", Email: "bob@example.com"},
		},
	}
	r2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(url.Values{
		"owner":           {"Alice"},
		"assignee_lookup": {"bob@example.com"},
	}.Encode()))
	r2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r2.ParseForm(); err != nil {
		t.Fatalf("parse form: %v", err)
	}
	owner2, assignee2 := parseMeasureAssignee(r2, q)
	if owner2 != "Alice" || !assignee2.Valid || assignee2.UUID != uid {
		t.Fatalf("unexpected assignee lookup resolution: owner=%q assignee=%v", owner2, assignee2)
	}
}
