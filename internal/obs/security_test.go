package obs

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

// captureDefaultLogger temporarily replaces slog.Default with a JSON handler
// writing to buf. Call this at the start of any test that triggers SecurityEvent
// or Logger output.
func captureDefaultLogger(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{
		Level:       slog.LevelDebug,
		ReplaceAttr: dropTimeAndLevel, // reuse helper from redact_test.go
	})
	old := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

// ── HashToken ─────────────────────────────────────────────────────────────────

func TestHashToken_EmptyKey(t *testing.T) {
	if got := HashToken("", "anything"); got != "" {
		t.Errorf("expected empty string for empty key, got %q", got)
	}
}

func TestHashToken_Deterministic(t *testing.T) {
	h1 := HashToken("key", "tok")
	h2 := HashToken("key", "tok")
	if h1 != h2 {
		t.Errorf("HashToken is not deterministic: %q vs %q", h1, h2)
	}
}

func TestHashToken_DifferentKeys(t *testing.T) {
	h1 := HashToken("key1", "tok")
	h2 := HashToken("key2", "tok")
	if h1 == h2 {
		t.Error("different keys should produce different hashes")
	}
}

func TestHashToken_DifferentTokens(t *testing.T) {
	h1 := HashToken("key", "tokenA")
	h2 := HashToken("key", "tokenB")
	if h1 == h2 {
		t.Error("different tokens should produce different hashes")
	}
}

func TestHashToken_IsHex(t *testing.T) {
	h := HashToken("key", "tok")
	if len(h) != 64 {
		t.Errorf("expected 64-char hex string (SHA-256), got len=%d %q", len(h), h)
	}
	for _, c := range h {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t.Errorf("non-hex character %q in hash %q", c, h)
		}
	}
}

func TestHashToken_NoRawTokenInOutput(t *testing.T) {
	// The raw token must not appear in the hash; this would indicate hashing was skipped.
	const tok = "super-secret-session-token-value"
	h := HashToken("k", tok)
	if h == tok {
		t.Error("HashToken returned the raw token — hashing was not applied")
	}
}

// ── SecurityEvent ─────────────────────────────────────────────────────────────

func TestSecurityEvent_LogType(t *testing.T) {
	buf := captureDefaultLogger(t)
	SecurityEvent(context.Background(), "auth.login")
	m := decodeOne(t, buf)
	if m["log_type"] != "security" {
		t.Errorf("log_type: got %v, want %q", m["log_type"], "security")
	}
}

func TestSecurityEvent_EventField(t *testing.T) {
	buf := captureDefaultLogger(t)
	SecurityEvent(context.Background(), "authz.denied")
	m := decodeOne(t, buf)
	if m["event"] != "authz.denied" {
		t.Errorf("event field: got %v, want %q", m["event"], "authz.denied")
	}
}

func TestSecurityEvent_ExtraAttrs(t *testing.T) {
	buf := captureDefaultLogger(t)
	SecurityEvent(context.Background(), "auth.login",
		"provider", "github",
		"user_id", "u-123")
	m := decodeOne(t, buf)
	if m["provider"] != "github" {
		t.Errorf("provider: got %v, want %q", m["provider"], "github")
	}
	if m["user_id"] != "u-123" {
		t.Errorf("user_id: got %v, want %q", m["user_id"], "u-123")
	}
}

func TestSecurityEvent_LevelIsWarn(t *testing.T) {
	var buf bytes.Buffer
	// Use a handler that preserves the level field.
	h := slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	old := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(old) })

	SecurityEvent(context.Background(), "auth.login")

	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if m["level"] != "WARN" {
		t.Errorf("expected level WARN, got %v", m["level"])
	}
}

func TestSecurityEvent_RawTokenNotLogged(t *testing.T) {
	const tok = "rawsessiontoken12345"
	buf := captureDefaultLogger(t)
	SecurityEvent(context.Background(), "auth.logout",
		"session_token_hash", HashToken("key", tok))
	out := buf.String()
	if bytes.Contains([]byte(out), []byte(tok)) {
		t.Errorf("raw token appeared in SecurityEvent output: %s", out)
	}
}
