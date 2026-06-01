package audit

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/obs"
	"github.com/3lbits/vigil/internal/testutil"
)

// ── test doubles ──────────────────────────────────────────────────────────────

type recordingQuerier struct {
	testutil.StubQuerier
	calls []db.InsertAuditLogParams
	err   error
}

func (q *recordingQuerier) InsertAuditLog(_ context.Context, p db.InsertAuditLogParams) error {
	q.calls = append(q.calls, p)
	return q.err
}

// ── helpers ───────────────────────────────────────────────────────────────────

// captureLogger replaces slog.Default with a buffer-backed JSON handler and
// returns the buffer. The previous logger is restored via t.Cleanup.
func captureLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	old := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

// decodeLastLine decodes the last non-empty JSON line from buf.
func decodeLastLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
	for i := len(lines) - 1; i >= 0; i-- {
		if len(bytes.TrimSpace(lines[i])) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(lines[i], &m); err != nil {
			t.Fatalf("decodeLastLine json.Unmarshal: %v (line: %q)", err, lines[i])
		}
		return m
	}
	t.Fatal("no JSON lines in buffer")
	return nil
}

// ── Record tests ──────────────────────────────────────────────────────────────

func TestRecord_CallsInsertAuditLog(t *testing.T) {
	captureLogger(t)
	q := &recordingQuerier{}

	err := Record(context.Background(), q, Event{Event: "test.event"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(q.calls) != 1 {
		t.Fatalf("expected 1 InsertAuditLog call, got %d", len(q.calls))
	}
	if q.calls[0].Event != "test.event" {
		t.Errorf("event: got %q, want %q", q.calls[0].Event, "test.event")
	}
}

func TestRecord_EmitsSlogLine(t *testing.T) {
	buf := captureLogger(t)
	q := &recordingQuerier{}

	Record(context.Background(), q, Event{Event: "admin.role_change"}) //nolint:errcheck,gosec

	m := decodeLastLine(t, buf)
	if m["log_type"] != "audit" {
		t.Errorf("log_type: got %v, want %q", m["log_type"], "audit")
	}
	if m["event"] != "admin.role_change" {
		t.Errorf("event: got %v, want %q", m["event"], "admin.role_change")
	}
}

func TestRecord_DBError_StillLogsToSlog(t *testing.T) {
	buf := captureLogger(t)
	q := &recordingQuerier{err: errors.New("db down")}

	err := Record(context.Background(), q, Event{Event: "auth.login"})
	if err == nil {
		t.Fatal("expected error from DB, got nil")
	}

	// Slog line must still have been emitted despite the DB error.
	m := decodeLastLine(t, buf)
	if m["log_type"] != "audit" {
		t.Errorf("slog not emitted on DB error; log_type=%v", m["log_type"])
	}
}

func TestRecord_NilAttrs_ValidJSON(t *testing.T) {
	captureLogger(t)
	q := &recordingQuerier{}

	Record(context.Background(), q, Event{Event: "x", Attrs: nil}) //nolint:errcheck,gosec

	if len(q.calls) == 0 {
		t.Fatal("no InsertAuditLog call")
	}
	var parsed any
	if err := json.Unmarshal(q.calls[0].Attrs, &parsed); err != nil {
		t.Errorf("nil Attrs should produce valid JSON; got unmarshal error: %v", err)
	}
}

func TestRecord_AttrsSerialised(t *testing.T) {
	captureLogger(t)
	q := &recordingQuerier{}

	Record(context.Background(), q, Event{ //nolint:errcheck,gosec
		Event: "x",
		Attrs: map[string]any{"target_user_id": "u-42", "new_role": "editor"},
	})

	if len(q.calls) == 0 {
		t.Fatal("no InsertAuditLog call")
	}
	var m map[string]any
	if err := json.Unmarshal(q.calls[0].Attrs, &m); err != nil {
		t.Fatalf("attrs JSON: %v", err)
	}
	if m["new_role"] != "editor" {
		t.Errorf("attrs.new_role: got %v, want %q", m["new_role"], "editor")
	}
}

func TestRecord_NoUserInContext_ZeroUUID(t *testing.T) {
	captureLogger(t)
	q := &recordingQuerier{}

	// No SessionUser injected → UserID must be null (Valid == false).
	Record(context.Background(), q, Event{Event: "x"}) //nolint:errcheck,gosec

	if len(q.calls) == 0 {
		t.Fatal("no InsertAuditLog call")
	}
	if q.calls[0].UserID.Valid {
		t.Errorf("expected UserID.Valid == false for unauthenticated context, got true (UUID=%v)",
			q.calls[0].UserID.UUID)
	}
}

func TestRecord_UserInContext_PopulatesUserID(t *testing.T) {
	captureLogger(t)
	q := &recordingQuerier{}

	ctx := middleware.SetUser(context.Background(), middleware.SessionUser{
		ID: "11111111-1111-1111-1111-111111111111", Role: "admin",
	})

	Record(ctx, q, Event{Event: "x"}) //nolint:errcheck,gosec

	if len(q.calls) == 0 {
		t.Fatal("no InsertAuditLog call")
	}
	if !q.calls[0].UserID.Valid {
		t.Error("expected UserID.Valid == true when user is in context")
	}
	if q.calls[0].UserID.UUID.String() != "11111111-1111-1111-1111-111111111111" {
		t.Errorf("user ID mismatch: %v", q.calls[0].UserID.UUID)
	}
}

func TestRecord_SourceIPPopulated(t *testing.T) {
	captureLogger(t)
	q := &recordingQuerier{}

	ctx := injectSourceIP(context.Background(), "10.0.0.1")
	Record(ctx, q, Event{Event: "x"}) //nolint:errcheck,gosec

	if len(q.calls) == 0 {
		t.Fatal("no InsertAuditLog call")
	}
	if q.calls[0].SourceIp != "10.0.0.1" {
		t.Errorf("source_ip: got %q, want %q", q.calls[0].SourceIp, "10.0.0.1")
	}
}

// ── helpers for context injection in tests ───────────────────────────────────

// injectSourceIP builds a context with the source IP by running SourceIPMiddleware
// on a synthetic request and capturing the enriched context.
func injectSourceIP(parent context.Context, ip string) context.Context {
	var result context.Context
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		result = r.Context() //nolint:fatcontext
	})
	r, _ := http.NewRequestWithContext(parent, http.MethodGet, "/", nil)
	r.RemoteAddr = ip + ":0"
	obs.SourceIPMiddleware(inner).ServeHTTP(httptest.NewRecorder(), r)
	return result
}

// ── RecordOrWarn ──────────────────────────────────────────────────────────────

func TestRecordOrWarn_SuccessDoesNotLog(t *testing.T) {
	buf := captureLogger(t)
	q := &recordingQuerier{}
	ctx := context.Background()

	RecordOrWarn(ctx, q, Event{
		Event: "test.event",
		Attrs: map[string]any{"id": "abc"},
	})

	if len(q.calls) != 1 {
		t.Fatalf("expected 1 InsertAuditLog call, got %d", len(q.calls))
	}
	// Record always emits one INFO line; it must not emit a WARN on success.
	m := decodeLastLine(t, buf)
	if level, _ := m["level"].(string); level == "WARN" {
		t.Errorf("expected no WARN log on success, got: %s", buf.String())
	}
}

func TestRecordOrWarn_DBErrorLogsWarning(t *testing.T) {
	buf := captureLogger(t)
	q := &recordingQuerier{err: errors.New("db down")}
	ctx := context.Background()

	RecordOrWarn(ctx, q, Event{
		Event: "test.event",
	})

	logOutput := buf.String()
	if logOutput == "" {
		t.Fatal("expected a warning log when DB write fails, got nothing")
	}
	m := decodeLastLine(t, buf)
	if level, _ := m["level"].(string); level != "WARN" {
		t.Errorf("expected WARN log level, got %q", level)
	}
}
