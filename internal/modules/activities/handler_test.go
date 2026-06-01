package activities

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/3lbits/vigil/internal/authz"
	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/testutil"
	"github.com/google/uuid"
)

// ── test double ───────────────────────────────────────────────────────────────

type activitiesQ struct {
	testutil.StubQuerier
	overdueErr   error
	filterRows   []db.FilterActivitiesRow
	filterActErr error
	activities   []db.ListActivitiesRow
	listActErr   error
	activity     db.GetActivityRow
	getActErr    error
	completed    db.Activity
	completeErr  error
	createCalls  []db.CreateActivityParams
	createErr    error
	measures     []db.Measure
	listMeasErr  error
	lookupUser   db.User
	lookupErr    error
}

func (q *activitiesQ) MarkOverdueActivities(_ context.Context) error {
	return q.overdueErr
}
func (q *activitiesQ) FilterActivities(_ context.Context, _ db.FilterActivitiesParams) ([]db.FilterActivitiesRow, error) {
	return q.filterRows, q.filterActErr
}
func (q *activitiesQ) ListActivities(_ context.Context) ([]db.ListActivitiesRow, error) {
	return q.activities, q.listActErr
}
func (q *activitiesQ) GetActivity(_ context.Context, _ uuid.UUID) (db.GetActivityRow, error) {
	return q.activity, q.getActErr
}
func (q *activitiesQ) CompleteActivity(_ context.Context, _ db.CompleteActivityParams) (db.Activity, error) {
	return q.completed, q.completeErr
}
func (q *activitiesQ) CreateActivity(_ context.Context, p db.CreateActivityParams) (db.Activity, error) {
	q.createCalls = append(q.createCalls, p)
	return db.Activity{}, q.createErr
}
func (q *activitiesQ) ListMeasures(_ context.Context) ([]db.Measure, error) {
	return q.measures, q.listMeasErr
}
func (q *activitiesQ) GetUserByID(_ context.Context, _ uuid.UUID) (db.User, error) {
	return q.lookupUser, q.lookupErr
}

// ── helpers ───────────────────────────────────────────────────────────────────

func adminCtx(r *http.Request) *http.Request {
	ctx := middleware.SetUser(r.Context(), middleware.SessionUser{
		ID: "00000000-0000-0000-0000-000000000001", Name: "Alice", Role: "admin",
	})
	return r.WithContext(ctx)
}

func newEngine(t *testing.T) *authz.Engine {
	t.Helper()
	e, err := authz.New(context.Background(), `
package authz
import rego.v1
default allow := false
allow if { input.user.role == "admin" }
`)
	if err != nil {
		t.Fatalf("compile policy: %v", err)
	}
	return e
}

func newHandlerT(t *testing.T, q *activitiesQ) *Handler {
	t.Helper()
	return NewHandler(q, newEngine(t))
}

// ── nextDueDate ───────────────────────────────────────────────────────────────

func TestNextDueDate_Monthly(t *testing.T) {
	from := time.Date(2025, 1, 15, 0, 0, 0, 0, time.UTC)
	got := nextDueDate("monthly", from)
	want := time.Date(2025, 2, 15, 0, 0, 0, 0, time.UTC)
	if !got.Valid || !got.Time.Equal(want) {
		t.Errorf("monthly: got %v, want %v", got.Time, want)
	}
}

func TestNextDueDate_Tertially(t *testing.T) {
	from := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	got := nextDueDate("tertially", from)
	want := time.Date(2025, 5, 1, 0, 0, 0, 0, time.UTC)
	if !got.Valid || !got.Time.Equal(want) {
		t.Errorf("tertially: got %v, want %v", got.Time, want)
	}
}

func TestNextDueDate_Annual(t *testing.T) {
	from := time.Date(2025, 6, 30, 0, 0, 0, 0, time.UTC)
	got := nextDueDate("annual", from)
	want := time.Date(2026, 6, 30, 0, 0, 0, 0, time.UTC)
	if !got.Valid || !got.Time.Equal(want) {
		t.Errorf("annual: got %v, want %v", got.Time, want)
	}
}

func TestNextDueDate_None(t *testing.T) {
	got := nextDueDate("none", time.Now())
	if got.Valid {
		t.Errorf("none: expected invalid NullTime, got %v", got.Time)
	}
}

func TestNextDueDate_Unknown(t *testing.T) {
	got := nextDueDate("weekly", time.Now())
	if got.Valid {
		t.Errorf("unknown recurrence: expected invalid NullTime, got %v", got.Time)
	}
}

// ── parseDateInput ────────────────────────────────────────────────────────────

func TestParseDateInput_ValidDate(t *testing.T) {
	got := parseDateInput("2025-03-15")
	if !got.Valid {
		t.Fatal("expected valid NullTime for well-formed date")
	}
	if got.Time.Year() != 2025 || got.Time.Month() != 3 || got.Time.Day() != 15 {
		t.Errorf("unexpected date: %v", got.Time)
	}
}

func TestParseDateInput_Empty(t *testing.T) {
	got := parseDateInput("")
	if got.Valid {
		t.Error("empty string should produce invalid NullTime")
	}
}

func TestParseDateInput_Whitespace(t *testing.T) {
	got := parseDateInput("   ")
	if got.Valid {
		t.Error("whitespace-only should produce invalid NullTime")
	}
}

func TestParseDateInput_WrongFormat(t *testing.T) {
	got := parseDateInput("15/03/2025")
	if got.Valid {
		t.Error("wrong date format should produce invalid NullTime")
	}
}

func TestParseDateInput_InvalidCalendar(t *testing.T) {
	got := parseDateInput("2025-13-01")
	if got.Valid {
		t.Error("invalid calendar date should produce invalid NullTime")
	}
}

// ── maybeCreateNextRecurring ──────────────────────────────────────────────────

func TestMaybeCreateNextRecurring_RecurringMonthly_CallsCreate(t *testing.T) {
	q := &activitiesQ{}
	h := newHandlerT(t, q)

	completed := db.Activity{
		ID:           uuid.New(),
		ActivityType: "recurring",
		Recurrence:   "monthly",
		Title:        "Monthly review",
	}
	h.maybeCreateNextRecurring(context.Background(), completed)

	if len(q.createCalls) != 1 {
		t.Fatalf("expected 1 CreateActivity call, got %d", len(q.createCalls))
	}
	if q.createCalls[0].Title != "Monthly review" {
		t.Errorf("created activity title: got %q, want %q", q.createCalls[0].Title, "Monthly review")
	}
	if q.createCalls[0].Recurrence != "monthly" {
		t.Errorf("created activity recurrence: got %q", q.createCalls[0].Recurrence)
	}
}

func TestMaybeCreateNextRecurring_OneOff_SkipsCreate(t *testing.T) {
	q := &activitiesQ{}
	h := newHandlerT(t, q)

	completed := db.Activity{ActivityType: "one_off", Recurrence: "none"}
	h.maybeCreateNextRecurring(context.Background(), completed)

	if len(q.createCalls) != 0 {
		t.Errorf("one_off activity should not spawn a next instance, got %d calls", len(q.createCalls))
	}
}

func TestMaybeCreateNextRecurring_RecurringNone_SkipsCreate(t *testing.T) {
	q := &activitiesQ{}
	h := newHandlerT(t, q)

	completed := db.Activity{ActivityType: "recurring", Recurrence: "none"}
	h.maybeCreateNextRecurring(context.Background(), completed)

	if len(q.createCalls) != 0 {
		t.Errorf("recurring+none should not spawn a next instance, got %d calls", len(q.createCalls))
	}
}

func TestMaybeCreateNextRecurring_DBError_DoesNotPropagrate(t *testing.T) {
	q := &activitiesQ{createErr: errors.New("db down")}
	h := newHandlerT(t, q)

	// Should not panic or propagate the error.
	completed := db.Activity{ActivityType: "recurring", Recurrence: "tertially"}
	h.maybeCreateNextRecurring(context.Background(), completed)
	// If we reach here without panic, the test passes.
}

// ── parseActivityAssignee ─────────────────────────────────────────────────────

func TestParseActivityAssignee_EmptyAssigneeID_ReturnsOwnerOnly(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		url.Values{"owner": {"Alice"}, "assignee_id": {""}}.Encode(),
	))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}

	q := &activitiesQ{}
	owner, assignee := parseActivityAssignee(r, q)

	if owner != "Alice" {
		t.Errorf("owner: got %q, want %q", owner, "Alice")
	}
	if assignee.Valid {
		t.Error("assigneeID should be invalid when assignee_id is empty")
	}
}

func TestParseActivityAssignee_ValidUUID_DBLookupSuccess_OverridesOwner(t *testing.T) {
	uid := uuid.New()
	q := &activitiesQ{lookupUser: db.User{Name: "Bob (from DB)"}, lookupErr: nil}

	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		url.Values{"owner": {"Alice"}, "assignee_id": {uid.String()}}.Encode(),
	))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}

	owner, assignee := parseActivityAssignee(r, q)

	if owner != "Bob (from DB)" {
		t.Errorf("owner should be overridden by DB lookup, got %q", owner)
	}
	if !assignee.Valid || assignee.UUID != uid {
		t.Errorf("assigneeID should be set to %v, got %v", uid, assignee)
	}
}

func TestParseActivityAssignee_ValidUUID_DBLookupFails_KeepsFormOwner(t *testing.T) {
	uid := uuid.New()
	q := &activitiesQ{lookupErr: errors.New("not found")}

	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		url.Values{"owner": {"Alice"}, "assignee_id": {uid.String()}}.Encode(),
	))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}

	owner, assignee := parseActivityAssignee(r, q)

	if owner != "Alice" {
		t.Errorf("owner should fall back to form value on DB error, got %q", owner)
	}
	if !assignee.Valid || assignee.UUID != uid {
		t.Errorf("assigneeID should still be set even when DB lookup fails, got %v", assignee)
	}
}

func TestParseActivityAssignee_InvalidUUID_ReturnsNullAssignee(t *testing.T) {
	q := &activitiesQ{}
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
		url.Values{"owner": {"Alice"}, "assignee_id": {"not-a-uuid"}}.Encode(),
	))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if err := r.ParseForm(); err != nil {
		t.Fatal(err)
	}

	_, assignee := parseActivityAssignee(r, q)

	if assignee.Valid {
		t.Error("invalid UUID should produce null assigneeID")
	}
}

// ── List handler ──────────────────────────────────────────────────────────────

func TestList_MarkOverdueError_StillReturns200(t *testing.T) {
	q := &activitiesQ{overdueErr: errors.New("db down")}
	h := newHandlerT(t, q)

	r := httptest.NewRequest(http.MethodGet, "/activities", nil)
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("MarkOverdueActivities error should be silently discarded; expected 200, got %d", w.Code)
	}
}

func TestList_DBError_Returns500(t *testing.T) {
	q := &activitiesQ{filterActErr: errors.New("db down")}
	h := newHandlerT(t, q)

	r := httptest.NewRequest(http.MethodGet, "/activities", nil)
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.List(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("ListActivities error: expected 500, got %d", w.Code)
	}
}

// ── Complete handler ──────────────────────────────────────────────────────────

func completeRequest(t *testing.T, id uuid.UUID, values url.Values) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/activities/"+id.String()+"/complete",
		strings.NewReader(values.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", id.String())
	return adminCtx(r)
}

func TestComplete_ValidHTTPSURL_Succeeds(t *testing.T) {
	id := uuid.New()
	q := &activitiesQ{completed: db.Activity{ID: id}}
	h := newHandlerT(t, q)

	r := completeRequest(t, id, url.Values{
		"evidence_url": {"https://example.com/evidence"},
		"completed_by": {"Alice"},
	})
	w := httptest.NewRecorder()
	h.Complete(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("valid https URL: expected 303, got %d", w.Code)
	}
	if strings.Contains(w.Header().Get("Location"), "error") {
		t.Error("valid https URL should not produce error flash")
	}
}

func TestComplete_ValidHTTPURL_Succeeds(t *testing.T) {
	id := uuid.New()
	q := &activitiesQ{completed: db.Activity{ID: id}}
	h := newHandlerT(t, q)

	r := completeRequest(t, id, url.Values{
		"evidence_url": {"http://internal.corp/ticket/123"},
	})
	w := httptest.NewRecorder()
	h.Complete(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("valid http URL: expected 303, got %d", w.Code)
	}
}

func TestComplete_InvalidURLScheme_RedirectsWithError(t *testing.T) {
	id := uuid.New()
	q := &activitiesQ{}
	h := newHandlerT(t, q)

	r := completeRequest(t, id, url.Values{
		"evidence_url": {"ftp://files.example.com/evidence.zip"},
	})
	w := httptest.NewRecorder()
	h.Complete(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("ftp URL: expected 303 redirect, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "error") {
		t.Error("invalid URL scheme should produce error flash")
	}
}

func TestComplete_InvalidURLMissingHost_RedirectsWithError(t *testing.T) {
	id := uuid.New()
	q := &activitiesQ{}
	h := newHandlerT(t, q)

	r := completeRequest(t, id, url.Values{
		"evidence_url": {"https:///evidence-only-path"},
	})
	w := httptest.NewRecorder()
	h.Complete(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("URL without host: expected 303 redirect, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "error") {
		t.Error("URL without host should produce error flash")
	}
}

func TestComplete_EmptyURL_Succeeds(t *testing.T) {
	id := uuid.New()
	q := &activitiesQ{completed: db.Activity{ID: id}}
	h := newHandlerT(t, q)

	r := completeRequest(t, id, url.Values{"evidence_url": {""}, "completed_by": {"Bob"}})
	w := httptest.NewRecorder()
	h.Complete(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("empty URL (allowed): expected 303, got %d", w.Code)
	}
}

func TestComplete_DBError_RedirectsWithError(t *testing.T) {
	id := uuid.New()
	q := &activitiesQ{completeErr: errors.New("db down")}
	h := newHandlerT(t, q)

	r := completeRequest(t, id, url.Values{"completed_by": {"Alice"}})
	w := httptest.NewRecorder()
	h.Complete(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("DB error: expected 303 redirect, got %d", w.Code)
	}
	if !strings.Contains(w.Header().Get("Location"), "error") {
		t.Error("DB error should produce error flash")
	}
}

func TestComplete_RecurringActivity_SpawnsNextInstance(t *testing.T) {
	id := uuid.New()
	q := &activitiesQ{
		completed: db.Activity{
			ID:           id,
			ActivityType: "recurring",
			Recurrence:   "monthly",
			Title:        "Monthly backup check",
			MeasureID:    uuid.NullUUID{},
		},
	}
	h := newHandlerT(t, q)

	r := completeRequest(t, id, url.Values{"completed_by": {"Alice"}})
	w := httptest.NewRecorder()
	h.Complete(w, r)

	if w.Code != http.StatusSeeOther {
		t.Errorf("expected 303, got %d", w.Code)
	}
	// CreateActivity should have been called once to spawn the next instance.
	// Note: first CreateActivity call is the complete+next-spawn; we count any call.
	if len(q.createCalls) < 1 {
		t.Error("recurring activity should spawn a next instance via CreateActivity")
	}
}

func TestComplete_InvalidID_Returns404(t *testing.T) {
	h := newHandlerT(t, &activitiesQ{})

	r := httptest.NewRequest(http.MethodPost, "/activities/bad-id/complete", nil)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	r.SetPathValue("id", "bad-id")
	r = adminCtx(r)
	w := httptest.NewRecorder()

	h.Complete(w, r)

	if w.Code != http.StatusNotFound {
		t.Errorf("invalid ID: expected 404, got %d", w.Code)
	}
}

// ── UpdateMeasureLastVerified called when measure linked ──────────────────────

type trackingQ struct {
	activitiesQ
	lastVerifiedCalls int
}

func (q *trackingQ) CompleteActivity(_ context.Context, _ db.CompleteActivityParams) (db.Activity, error) {
	return q.completed, q.completeErr
}

func (q *trackingQ) UpdateMeasureLastVerified(_ context.Context, _ uuid.UUID) error {
	q.lastVerifiedCalls++
	return nil
}

func (q *trackingQ) CreateActivity(_ context.Context, p db.CreateActivityParams) (db.Activity, error) {
	q.createCalls = append(q.createCalls, p)
	return db.Activity{}, nil
}

func (q *trackingQ) MarkOverdueActivities(_ context.Context) error { return nil }
func (q *trackingQ) ListActivities(_ context.Context) ([]db.ListActivitiesRow, error) {
	return nil, nil
}
func (q *trackingQ) ListMeasures(_ context.Context) ([]db.Measure, error) { return nil, nil }
func (q *trackingQ) GetUserByID(_ context.Context, _ uuid.UUID) (db.User, error) {
	return db.User{}, errors.New("not found")
}

func TestComplete_MeasureLinked_UpdatesLastVerified(t *testing.T) {
	id := uuid.New()
	mid := uuid.New()
	q := &trackingQ{
		activitiesQ: activitiesQ{
			completed: db.Activity{
				ID:           id,
				MeasureID:    uuid.NullUUID{UUID: mid, Valid: true},
				ActivityType: "one_off",
				Recurrence:   "none",
			},
		},
	}
	e, _ := authz.New(context.Background(), `
package authz
import rego.v1
default allow := false
allow if { input.user.role == "admin" }
`)
	h := NewHandler(q, e)

	r := completeRequest(t, id, url.Values{"completed_by": {"Alice"}})
	w := httptest.NewRecorder()
	h.Complete(w, r)

	if q.lastVerifiedCalls != 1 {
		t.Errorf("UpdateMeasureLastVerified should be called once for linked measure, got %d", q.lastVerifiedCalls)
	}
}

func TestComplete_NoMeasureLinked_SkipsLastVerified(t *testing.T) {
	id := uuid.New()
	q := &trackingQ{
		activitiesQ: activitiesQ{
			completed: db.Activity{
				ID:           id,
				MeasureID:    uuid.NullUUID{Valid: false}, // no measure
				ActivityType: "one_off",
				Recurrence:   "none",
			},
		},
	}
	e, _ := authz.New(context.Background(), `
package authz
import rego.v1
default allow := false
allow if { input.user.role == "admin" }
`)
	h := NewHandler(q, e)

	r := completeRequest(t, id, url.Values{"completed_by": {"Alice"}})
	w := httptest.NewRecorder()
	h.Complete(w, r)

	if q.lastVerifiedCalls != 0 {
		t.Errorf("UpdateMeasureLastVerified should not be called when no measure is linked, got %d", q.lastVerifiedCalls)
	}
}

func TestRejectEnum(t *testing.T) {
	w := httptest.NewRecorder()
	if rejectEnum(w, "kind", "", "a,b", map[string]bool{"a": true}) {
		t.Fatal("empty value should not be rejected")
	}
	w = httptest.NewRecorder()
	if rejectEnum(w, "kind", "a", "a,b", map[string]bool{"a": true}) {
		t.Fatal("allowed value should not be rejected")
	}
	w = httptest.NewRecorder()
	if !rejectEnum(w, "kind", "x", "a,b", map[string]bool{"a": true}) {
		t.Fatal("invalid value should be rejected")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestParseActivityEnums(t *testing.T) {
	w := httptest.NewRecorder()
	actType, recurrence, priority, kind, ok := parseActivityEnums(w, "", "", "", "")
	if !ok {
		t.Fatal("expected defaults to pass")
	}
	if actType != "one_off" || recurrence != "none" || priority != "medium" || kind != "review" {
		t.Fatalf("unexpected defaults: %q %q %q %q", actType, recurrence, priority, kind)
	}

	w = httptest.NewRecorder()
	_, _, _, _, ok = parseActivityEnums(w, "invalid", "", "", "")
	if ok {
		t.Fatal("invalid activity_type should fail")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// ── sql.NullTime zero value helper ───────────────────────────────────────────

var _ = sql.NullTime{} // keep import used
