package obs

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
)

// SecurityEvent logs a security-relevant event at WARN level with log_type=security.
// It automatically attaches trace_id, span_id, request_id, source_ip, and user.id
// from ctx (via Logger). Callers supply event-specific attrs as key-value pairs.
//
// Session tokens must be hashed before passing as attr values; use HashToken.
func SecurityEvent(ctx context.Context, event string, attrs ...any) {
	args := make([]any, 0, 4+len(attrs))
	args = append(args, "log_type", "security", "event", event)
	args = append(args, attrs...)
	Logger(ctx).WarnContext(ctx, event, args...)
}

// HashToken returns the HMAC-SHA256 of token keyed with key, encoded as hex.
// Returns "" when key is empty so callers can omit the attr gracefully rather
// than leak raw tokens when SESSION_HMAC_KEY is not configured.
func HashToken(key, token string) string {
	if key == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(token)) // #nosec G104 — hmac.Hash.Write never returns an error
	return hex.EncodeToString(mac.Sum(nil))
}
