package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"
)

// newTestHandler returns a redactHandler whose inner JSON handler writes to buf.
func newTestHandler(buf *bytes.Buffer) slog.Handler {
	inner := slog.NewJSONHandler(buf, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		ReplaceAttr: dropTimeAndLevel, // keep output stable for assertions
	})
	return NewRedactHandler(inner)
}

// dropTimeAndLevel strips time and level so test output only contains the fields
// we care about.
func dropTimeAndLevel(_ []string, a slog.Attr) slog.Attr {
	if a.Key == slog.TimeKey || a.Key == slog.LevelKey {
		return slog.Attr{}
	}
	return a
}

// logRecord emits a single record through handler. Callers read the buffer directly.
func logRecord(t *testing.T, handler slog.Handler, attrs ...slog.Attr) {
	t.Helper()
	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "test", 0)
	r.AddAttrs(attrs...)
	if err := handler.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}
}

// decodeOne decodes a single JSON line from buf.
func decodeOne(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("json.Unmarshal(%q): %v", buf.String(), err)
	}
	return m
}

// ── Deny-list: key-based redaction ────────────────────────────────────────────

func TestRedact_DenyKey_Direct(t *testing.T) {
	tests := []struct{ key, val string }{
		{"password", "s3cr3t"},
		{"token", "tok_live_abc123"},
		{"access_token", "at_xyz"},
		{"cookie", "vigil_session=abc"},
		{"session", "sess-id-123"},
		{"session_id", "sid-456"},
		{"authorization", "Basic dXNlcjpwYXNz"},
		{"secret", "top-secret"},
		{"client_secret", "cs_abc"},
		{"api_key", "key_live_123"},
		{"private_key", "-----BEGIN RSA"},
		{"ssn", "123-45-6789"},
		{"fnr", "01019012345"},
		{"credit_card", "4111111111111111"},
		{"cvv", "123"},
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			var buf bytes.Buffer
			h := newTestHandler(&buf)
			logRecord(t, h, slog.String(tc.key, tc.val))
			m := decodeOne(t, &buf)
			if got := m[tc.key]; got != redacted {
				t.Errorf("key %q: got %q, want %q", tc.key, got, redacted)
			}
		})
	}
}

func TestRedact_DenyKey_CaseInsensitive(t *testing.T) {
	for _, key := range []string{"Password", "PASSWORD", "pAsSwOrD"} {
		t.Run(key, func(t *testing.T) {
			var buf bytes.Buffer
			h := newTestHandler(&buf)
			logRecord(t, h, slog.String(key, "secret"))
			m := decodeOne(t, &buf)
			if got := m[key]; got != redacted {
				t.Errorf("key %q: got %q, want %q", key, got, redacted)
			}
		})
	}
}

func TestRedact_DenyKey_PartialNoMatch(t *testing.T) {
	// "passwords_policy" contains "password" but is not in the deny-list.
	var buf bytes.Buffer
	h := newTestHandler(&buf)
	const val = "allow-all"
	logRecord(t, h, slog.String("passwords_policy", val))
	m := decodeOne(t, &buf)
	if got := m["passwords_policy"]; got != val {
		t.Errorf("got %q, want %q (partial key should not be redacted)", got, val)
	}
}

// ── Safe key passthrough ───────────────────────────────────────────────────────

func TestRedact_SafeKey_Passthrough(t *testing.T) {
	tests := []struct {
		key string
		val string
	}{
		{"user_id", "u-abc123"},
		{"request_id", "req-xyz"},
		{"status", "ok"},
		{"count", "42"},
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			var buf bytes.Buffer
			h := newTestHandler(&buf)
			logRecord(t, h, slog.String(tc.key, tc.val))
			m := decodeOne(t, &buf)
			if got := m[tc.key]; got != tc.val {
				t.Errorf("key %q: got %q, want %q", tc.key, got, tc.val)
			}
		})
	}
}

// ── Regex scrubbing in string values ──────────────────────────────────────────

func TestRedact_Email_ScrubInValue(t *testing.T) {
	tests := []struct {
		name  string
		input string
		// expected contains [REDACTED] where the email was
		wantContains string
	}{
		{"plain", "contact alice@example.com for info", "[REDACTED]"},
		{"only_email", "alice@example.com", "[REDACTED]"},
		{"multiple_emails", "a@b.com and c@d.org", "[REDACTED]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := newTestHandler(&buf)
			logRecord(t, h, slog.String("note", tc.input))
			m := decodeOne(t, &buf)
			val, _ := m["note"].(string)
			if val == tc.input {
				t.Errorf("email was not scrubbed: %q", val)
			}
		})
	}
}

func TestRedact_Bearer_ScrubInValue(t *testing.T) {
	inputs := []string{
		"Bearer eyJhbGciOiJSUzI1NiJ9.payload.sig",
		"bearer abc123def456",
		"BEARER tok+/=",
	}
	for _, input := range inputs {
		t.Run(input[:10], func(t *testing.T) {
			var buf bytes.Buffer
			h := newTestHandler(&buf)
			logRecord(t, h, slog.String("hdr", input))
			m := decodeOne(t, &buf)
			val, _ := m["hdr"].(string)
			if val == input {
				t.Errorf("bearer token was not scrubbed: %q", val)
			}
		})
	}
}

func TestRedact_Fnr_ScrubInValue(t *testing.T) {
	// Norwegian fødselsnummer: exactly 11 digits at a word boundary.
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"standalone", "01019012345", "[REDACTED]"},
		{"in_sentence", "fnr is 01019012345 registered", "fnr is [REDACTED] registered"},
		{"no_match_10", "1234567890", "1234567890"},     // 10 digits — not an fnr
		{"no_match_12", "123456789012", "123456789012"}, // 12 digits — \b won't match mid-run
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			h := newTestHandler(&buf)
			logRecord(t, h, slog.String("ref", tc.input))
			m := decodeOne(t, &buf)
			got, _ := m["ref"].(string)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// ── Nested group recursion ─────────────────────────────────────────────────────

func TestRedact_NestedGroup_DenyKey(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf)
	logRecord(t, h,
		slog.Group("request",
			slog.String("method", "GET"),
			slog.String("password", "x"),
		),
	)
	m := decodeOne(t, &buf)
	req, ok := m["request"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'request' group in output, got %v", m)
	}
	if req["method"] != "GET" {
		t.Errorf("safe key 'method': got %v", req["method"])
	}
	if req["password"] != redacted {
		t.Errorf("deny key 'password': got %v, want %q", req["password"], redacted)
	}
}

func TestRedact_DeeplyNested_Group(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf)
	logRecord(t, h,
		slog.Group("outer",
			slog.Group("inner",
				slog.String("token", "tok123"),
				slog.String("ok", "fine"),
			),
		),
	)
	m := decodeOne(t, &buf)
	outer, _ := m["outer"].(map[string]any)
	inner, _ := outer["inner"].(map[string]any)
	if inner["token"] != redacted {
		t.Errorf("deeply nested token: got %v, want %q", inner["token"], redacted)
	}
	if inner["ok"] != "fine" {
		t.Errorf("safe nested key: got %v, want %q", inner["ok"], "fine")
	}
}

// ── LogValuer resolution ───────────────────────────────────────────────────────

// sensitiveValue implements slog.LogValuer and returns a string with an email.
type sensitiveValue struct{ email string }

func (s sensitiveValue) LogValue() slog.Value {
	return slog.StringValue("user email is " + s.email)
}

// denyKeyValue implements slog.LogValuer and returns a string value.
// Used to verify that resolution happens before regex scrubbing.
type denyKeyValue struct{ secret string }

func (d denyKeyValue) LogValue() slog.Value {
	return slog.StringValue(d.secret)
}

func TestRedact_LogValuer_EmailInResolvedValue(t *testing.T) {
	var buf bytes.Buffer
	h := newTestHandler(&buf)
	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "test", 0)
	r.AddAttrs(slog.Any("info", sensitiveValue{email: "alice@example.com"}))
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	m := decodeOne(t, &buf)
	val, _ := m["info"].(string)
	if val == "user email is alice@example.com" {
		t.Error("LogValuer email was not scrubbed after resolution")
	}
}

func TestRedact_LogValuer_DenyKeyResolved(t *testing.T) {
	// The attr key "secret" is on the deny-list; the LogValue should not matter.
	var buf bytes.Buffer
	h := newTestHandler(&buf)
	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "test", 0)
	r.AddAttrs(slog.Any("secret", denyKeyValue{secret: "top-secret-value"}))
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	m := decodeOne(t, &buf)
	if m["secret"] != redacted {
		t.Errorf("got %v, want %q", m["secret"], redacted)
	}
}

// ── WithAttrs ─────────────────────────────────────────────────────────────────

func TestRedact_WithAttrs_DenyKey(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		ReplaceAttr: dropTimeAndLevel,
	})
	h := NewRedactHandler(inner).WithAttrs([]slog.Attr{
		slog.String("token", "tok_live_abc"),
		slog.String("user_id", "u-123"),
	})

	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "test", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	m := decodeOne(t, &buf)
	if m["token"] != redacted {
		t.Errorf("WithAttrs token: got %v, want %q", m["token"], redacted)
	}
	if m["user_id"] != "u-123" {
		t.Errorf("WithAttrs user_id: got %v, want %q", m["user_id"], "u-123")
	}
}

// ── WithGroup + deny key ───────────────────────────────────────────────────────

func TestRedact_WithGroup_DenyKey(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		ReplaceAttr: dropTimeAndLevel,
	})
	// Simulate: logger.WithGroup("ctx").With("token", "x").Info("msg")
	h := NewRedactHandler(inner).WithGroup("ctx")
	h = h.WithAttrs([]slog.Attr{slog.String("token", "secret-token")})

	r := slog.NewRecord(time.Time{}, slog.LevelInfo, "test", 0)
	if err := h.Handle(context.Background(), r); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	m := decodeOne(t, &buf)
	grp, ok := m["ctx"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'ctx' group in output: %v", m)
	}
	if grp["token"] != redacted {
		t.Errorf("WithGroup token: got %v, want %q", grp["token"], redacted)
	}
}

// ── Enabled delegation ────────────────────────────────────────────────────────

func TestRedact_Enabled_DelegatesToInner(t *testing.T) {
	var buf bytes.Buffer
	inner := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn})
	h := NewRedactHandler(inner)

	if h.Enabled(context.Background(), slog.LevelDebug) {
		t.Error("LevelDebug should be disabled when inner level is Warn")
	}
	if h.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("LevelInfo should be disabled when inner level is Warn")
	}
	if !h.Enabled(context.Background(), slog.LevelWarn) {
		t.Error("LevelWarn should be enabled")
	}
	if !h.Enabled(context.Background(), slog.LevelError) {
		t.Error("LevelError should be enabled")
	}
}
