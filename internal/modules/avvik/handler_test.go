package avvik

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/testutil"
	"github.com/google/uuid"
)

type avvikQ struct {
	testutil.StubQuerier
	getErr error
}

func (q *avvikQ) GetAvvik(_ context.Context, _ uuid.UUID) (db.Avvik, error) {
	if q.getErr != nil {
		return db.Avvik{}, q.getErr
	}
	return db.Avvik{ID: uuid.New(), Title: "Test avvik"}, nil
}

type avvikQWithUsers struct {
	avvikQ
	users  []db.User
	events []db.AvvikEvent
}

func (q *avvikQWithUsers) ListUsers(_ context.Context) ([]db.User, error) {
	return q.users, nil
}

func (q *avvikQWithUsers) ListAvvikEvents(_ context.Context, _ uuid.UUID) ([]db.AvvikEvent, error) {
	return q.events, nil
}

func newTestHandler(q db.Querier) *Handler {
	return NewHandler(q, (*sql.DB)(nil), nil)
}

func TestList_ReturnsOK(t *testing.T) {
	h := newTestHandler(&avvikQ{})
	r := httptest.NewRequest(http.MethodGet, "/avvik", nil)
	w := httptest.NewRecorder()

	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestList_HTMX_ReturnsOK(t *testing.T) {
	h := newTestHandler(&avvikQ{})
	r := httptest.NewRequest(http.MethodGet, "/avvik", nil)
	r.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()

	h.List(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestNew_ReturnsOK(t *testing.T) {
	h := newTestHandler(&avvikQ{})
	r := httptest.NewRequest(http.MethodGet, "/avvik/new", nil)
	w := httptest.NewRecorder()

	h.New(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestShow_InvalidID_ReturnsNotFound(t *testing.T) {
	h := newTestHandler(&avvikQ{})
	r := httptest.NewRequest(http.MethodGet, "/avvik/not-a-uuid", nil)
	r.SetPathValue("id", "not-a-uuid")
	w := httptest.NewRecorder()

	h.Show(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestShow_UnknownID_ReturnsNotFound(t *testing.T) {
	h := newTestHandler(&avvikQ{getErr: errors.New("not found")})
	id := uuid.NewString()
	r := httptest.NewRequest(http.MethodGet, "/avvik/"+id, nil)
	r.SetPathValue("id", id)
	w := httptest.NewRecorder()

	h.Show(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestNullBoolFromQuery(t *testing.T) {
	tests := []struct {
		input     string
		wantValid bool
		wantBool  bool
	}{
		{"true", true, true},
		{"True", true, true},
		{"TRUE", true, true},
		{"1", true, true},
		{"yes", true, true},
		{"on", true, true},
		{"false", true, false},
		{"False", true, false},
		{"0", true, false},
		{"no", true, false},
		{"off", true, false},
		{"", false, false},
		{"maybe", false, false},
		{" true ", true, true},
	}
	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got := nullBoolFromQuery(tc.input)
			if got.Valid != tc.wantValid {
				t.Errorf("nullBoolFromQuery(%q).Valid = %v, want %v", tc.input, got.Valid, tc.wantValid)
			}
			if got.Valid && got.Bool != tc.wantBool {
				t.Errorf("nullBoolFromQuery(%q).Bool = %v, want %v", tc.input, got.Bool, tc.wantBool)
			}
		})
	}
}

func TestParseNullUUID(t *testing.T) {
	validID := uuid.MustParse("11111111-2222-3333-4444-555555555555")
	tests := []struct {
		name      string
		input     string
		wantValid bool
		wantUUID  uuid.UUID
	}{
		{"valid UUID", validID.String(), true, validID},
		{"valid UUID with whitespace", "  " + validID.String() + "  ", true, validID},
		{"empty string", "", false, uuid.Nil},
		{"invalid string", "not-a-uuid", false, uuid.Nil},
		{"partial UUID", "11111111-2222", false, uuid.Nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parseNullUUID(tc.input)
			if got.Valid != tc.wantValid {
				t.Errorf("parseNullUUID(%q).Valid = %v, want %v", tc.input, got.Valid, tc.wantValid)
			}
			if got.Valid && got.UUID != tc.wantUUID {
				t.Errorf("parseNullUUID(%q).UUID = %v, want %v", tc.input, got.UUID, tc.wantUUID)
			}
		})
	}
}

func TestCurrentUser(t *testing.T) {
	uid := uuid.New()
	ctx := middleware.SetUser(context.Background(), middleware.SessionUser{
		ID: uid.String(), Name: "Alice", Email: "alice@example.com", Role: "editor",
	})
	gotID, gotName := currentUser(ctx)
	if !gotID.Valid || gotID.UUID != uid || gotName != "Alice" {
		t.Fatalf("unexpected currentUser result: id=%v name=%q", gotID, gotName)
	}

	gotID, gotName = currentUser(context.Background())
	if gotID.Valid || gotName != "" {
		t.Fatalf("expected zero values without user context, got id=%v name=%q", gotID, gotName)
	}
}

func TestPathUUID(t *testing.T) {
	h := newTestHandler(&avvikQ{})
	req := httptest.NewRequest(http.MethodGet, "/avvik/x", nil)
	req.SetPathValue("id", "not-a-uuid")
	w := httptest.NewRecorder()
	if _, ok := h.pathUUID(w, req, "id"); ok {
		t.Fatal("expected parse failure")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/avvik/x", nil)
	id := uuid.New()
	req2.SetPathValue("id", id.String())
	w2 := httptest.NewRecorder()
	got, ok := h.pathUUID(w2, req2, "id")
	if !ok || got != id {
		t.Fatalf("expected parsed UUID %v, got %v (ok=%v)", id, got, ok)
	}
}

func TestParseDateAndTimeHelpers(t *testing.T) {
	if got := parseDateOrNow("2026-01-02"); got.Format("2006-01-02") != "2026-01-02" {
		t.Fatalf("unexpected parsed date: %v", got)
	}
	if got := parseDateOrNow("2026-01-02T10:00:00Z"); got.UTC().Format(time.RFC3339) != "2026-01-02T10:00:00Z" {
		t.Fatalf("unexpected parsed RFC3339: %v", got)
	}
	nowish := parseDateOrNow("bad")
	if time.Since(nowish) > 2*time.Second {
		t.Fatalf("invalid input should fallback to now, got %v", nowish)
	}

	if got := parseNullTimeRFC3339("2026-01-02T10:00:00Z"); !got.Valid {
		t.Fatal("expected valid RFC3339 null time")
	}
	if got := parseNullTimeRFC3339("bad"); got.Valid {
		t.Fatal("expected invalid RFC3339 null time for bad input")
	}
	if got := parseNullTimeDate("2026-01-02"); !got.Valid || got.Time.Format("2006-01-02") != "2026-01-02" {
		t.Fatalf("unexpected parsed date null time: %v", got)
	}
	if got := parseNullTimeDate("bad"); got.Valid {
		t.Fatal("expected invalid date null time for bad input")
	}
}

func TestSmallHelpers(t *testing.T) {
	if got := defaultIfEmpty("  ", "fallback"); got != "fallback" {
		t.Fatalf("defaultIfEmpty blank = %q", got)
	}
	if got := defaultIfEmpty(" value ", "fallback"); got != "value" {
		t.Fatalf("defaultIfEmpty trim = %q", got)
	}

	w := httptest.NewRecorder()
	if rejectEnum(w, "kind", "", "a,b", map[string]bool{"a": true}) {
		t.Fatal("empty enum should be accepted")
	}
	w = httptest.NewRecorder()
	if !rejectEnum(w, "kind", "x", "a,b", map[string]bool{"a": true}) || w.Code != http.StatusBadRequest {
		t.Fatal("invalid enum should be rejected with 400")
	}

	w = httptest.NewRecorder()
	actType, recurrence, priority, kind, ok := parseActivityEnums(w, "", "", "", "")
	if !ok || actType != "one_off" || recurrence != "none" || priority != "medium" || kind != "review" {
		t.Fatalf("unexpected defaults: %q %q %q %q (ok=%v)", actType, recurrence, priority, kind, ok)
	}
	w = httptest.NewRecorder()
	_, _, _, _, ok = parseActivityEnums(w, "", "weekly", "", "")
	if ok || w.Code != http.StatusBadRequest {
		t.Fatal("invalid recurrence should fail with 400")
	}
}

func TestCanCloseAvvik(t *testing.T) {
	allTrue := db.Avvik{
		LogQaDone: true, FollowupsDelegated: true, ReporterInformed: true, OrgInformed: true,
		DecisionsAnchored: true, ImplementationDeadlineSet: true,
	}
	if !canCloseAvvik(allTrue) {
		t.Fatal("expected closure when all mandatory flags are true")
	}
	high := allTrue
	high.RiskLevel = "high"
	if canCloseAvvik(high) {
		t.Fatal("high risk should require mgmt informed")
	}
	high.MgmtInformed = true
	if !canCloseAvvik(high) {
		t.Fatal("high risk should close when mgmt informed")
	}
}

func TestResolveReporterAndAssigneeLookup(t *testing.T) {
	u := db.User{ID: uuid.New(), Name: "Alice", Email: "alice@example.com"}
	q := &avvikQWithUsers{users: []db.User{u}}
	h := newTestHandler(q)

	name, email := h.resolveReporter(context.Background(), "alice@example.com", "")
	if name != "Alice" || email != "alice@example.com" {
		t.Fatalf("unexpected resolved reporter: %q %q", name, email)
	}
	name, email = h.resolveReporter(context.Background(), "Unknown", "")
	if name != "Unknown" || email != "" {
		t.Fatalf("unexpected fallback reporter: %q %q", name, email)
	}

	got := h.resolveUserLookupAssignee(context.Background(), "", "Alice")
	if !got.Valid || got.UUID != u.ID {
		t.Fatalf("unexpected assignee lookup result: %v", got)
	}
	got = h.resolveUserLookupAssignee(context.Background(), u.ID.String(), "ignored")
	if !got.Valid || got.UUID != u.ID {
		t.Fatalf("raw assignee ID should win: %v", got)
	}
	got = h.resolveUserLookupAssignee(context.Background(), "", "")
	if got.Valid {
		t.Fatalf("empty lookup should return null UUID, got %v", got)
	}
}

func TestCanReporterOrCreatorAccess(t *testing.T) {
	av := db.Avvik{ID: uuid.New(), ReporterEmail: "reporter@example.com"}
	h := newTestHandler(&avvikQWithUsers{})
	if !h.canReporterOrCreatorAccess(context.Background(), av, middleware.SessionUser{Role: "admin"}) {
		t.Fatal("admin should always have access")
	}
	if !h.canReporterOrCreatorAccess(context.Background(), av, middleware.SessionUser{Email: "reporter@example.com"}) {
		t.Fatal("matching reporter email should grant access")
	}
	if h.canReporterOrCreatorAccess(context.Background(), av, middleware.SessionUser{ID: "bad-id"}) {
		t.Fatal("invalid user id should deny access")
	}

	actorID := uuid.New()
	h2 := newTestHandler(&avvikQWithUsers{
		events: []db.AvvikEvent{{EventType: "created", ActorID: uuid.NullUUID{UUID: actorID, Valid: true}}},
	})
	if !h2.canReporterOrCreatorAccess(context.Background(), av, middleware.SessionUser{ID: actorID.String()}) {
		t.Fatal("creator should have access")
	}
}
