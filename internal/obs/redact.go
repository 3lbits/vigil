package obs

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
)

// redacted is the sentinel value substituted for suppressed content.
const redacted = "[REDACTED]"

// defaultDenyKeys lists attribute key names (lower-cased) whose values are
// always replaced with [REDACTED], regardless of their type.
var defaultDenyKeys = []string{
	"password", "passwd", "pass",
	"token", "access_token", "refresh_token", "id_token", "auth_token",
	"cookie", "session", "session_id", "session_token",
	"authorization",
	"secret", "client_secret", "api_key", "api_secret",
	"private_key", "private_key_id",
	"ssn", "fnr",
	"credit_card", "card_number", "cvv",
}

// defaultPatterns scrubs PII from string values via regex substitution.
// Applied after the deny-list check, so only non-denied string values are scanned.
var defaultPatterns = []*regexp.Regexp{
	// Email addresses.
	regexp.MustCompile(`[a-zA-Z0-9._%+\-]+@[a-zA-Z0-9.\-]+\.[a-zA-Z]{2,}`),
	// Bearer tokens in Authorization-style header values.
	regexp.MustCompile(`(?i)bearer\s+[a-zA-Z0-9\-._~+/]+=*`),
	// Norwegian fødselsnummer: 11 consecutive digits at a word boundary.
	regexp.MustCompile(`\b\d{11}\b`),
}

// redactHandler is an slog.Handler that strips PII before forwarding records
// to the wrapped inner handler.
type redactHandler struct {
	inner    slog.Handler
	denyKeys map[string]bool
	patterns []*regexp.Regexp
}

// NewRedactHandler wraps inner with the default deny-list and regex patterns.
func NewRedactHandler(inner slog.Handler) slog.Handler {
	keys := make(map[string]bool, len(defaultDenyKeys))
	for _, k := range defaultDenyKeys {
		keys[k] = true
	}
	return &redactHandler{
		inner:    inner,
		denyKeys: keys,
		patterns: defaultPatterns,
	}
}

// Enabled delegates to the inner handler — no overhead when the level is not met.
func (h *redactHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.inner.Enabled(ctx, lvl)
}

// Handle redacts the record's attributes then forwards to the inner handler.
func (h *redactHandler) Handle(ctx context.Context, r slog.Record) error {
	attrs := make([]slog.Attr, 0, r.NumAttrs())
	r.Attrs(func(a slog.Attr) bool {
		attrs = append(attrs, a)
		return true
	})
	clean := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	clean.AddAttrs(h.redactAttrs(attrs)...)
	return h.inner.Handle(ctx, clean) //nolint:wrapcheck
}

// WithAttrs redacts attrs before attaching them to the inner handler, so
// pre-attached (structured) fields are scrubbed at binding time.
func (h *redactHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &redactHandler{
		inner:    h.inner.WithAttrs(h.redactAttrs(attrs)),
		denyKeys: h.denyKeys,
		patterns: h.patterns,
	}
}

// WithGroup delegates group nesting to the inner handler.
// The denyKeys and patterns maps are read-only and safe to share.
func (h *redactHandler) WithGroup(name string) slog.Handler {
	return &redactHandler{
		inner:    h.inner.WithGroup(name),
		denyKeys: h.denyKeys,
		patterns: h.patterns,
	}
}

// redactAttrs applies redactAttr to each element of attrs.
func (h *redactHandler) redactAttrs(attrs []slog.Attr) []slog.Attr {
	out := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		out[i] = h.redactAttr(a)
	}
	return out
}

// redactAttr processes a single attribute:
//  1. Resolves any LogValuer so the concrete value is inspected.
//  2. Replaces the value with [REDACTED] when the key is on the deny-list.
//  3. Recurses into slog groups.
//  4. Applies regex scrubbing to string values.
func (h *redactHandler) redactAttr(a slog.Attr) slog.Attr {
	// Resolve LogValuer before any inspection.
	a.Value = a.Value.Resolve()

	// Key-based deny-list check (leaf key name, case-insensitive).
	if h.denyKeys[strings.ToLower(a.Key)] {
		return slog.Attr{Key: a.Key, Value: slog.StringValue(redacted)}
	}

	switch a.Value.Kind() {
	case slog.KindGroup:
		cleaned := h.redactAttrs(a.Value.Group())
		return slog.Attr{Key: a.Key, Value: slog.GroupValue(cleaned...)}
	case slog.KindString:
		return slog.Attr{Key: a.Key, Value: slog.StringValue(h.scrubString(a.Value.String()))}
	case slog.KindAny, slog.KindBool, slog.KindDuration, slog.KindFloat64,
		slog.KindInt64, slog.KindTime, slog.KindUint64, slog.KindLogValuer:
		return a
	}
	return a
}

// scrubString replaces every regex match in s with [REDACTED].
func (h *redactHandler) scrubString(s string) string {
	for _, re := range h.patterns {
		s = re.ReplaceAllString(s, redacted)
	}
	return s
}
