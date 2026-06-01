package httpresp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/3lbits/vigil/internal/locale"
)

// localeCtx returns a context with a localizer for the given language.
func localeCtx(t *testing.T, lang string) context.Context {
	t.Helper()
	bundle, err := locale.NewBundle()
	if err != nil {
		t.Fatalf("locale.NewBundle: %v", err)
	}
	loc := i18n.NewLocalizer(bundle, lang, locale.DefaultLang)
	return locale.SetLocalizer(context.Background(), loc, lang)
}

// ── Error: HTMX inline response ───────────────────────────────────────────────

func TestError_HTMX_WritesInlineFragment(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("HX-Request", "true")
	r = r.WithContext(localeCtx(t, locale.LangEN))
	w := httptest.NewRecorder()

	Error(w, r, http.StatusForbidden)

	if w.Code != http.StatusForbidden {
		t.Errorf("status: got %d, want 403", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "HTTP 403") {
		t.Errorf("body missing HTTP 403, got: %q", body)
	}
	// Must not be a full HTML page.
	if strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("HTMX response must not be a full page")
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type: got %q, want text/html prefix", ct)
	}
}

func TestError_HTMX_DoesNotIncludeRequestID(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("HX-Request", "true")
	r = r.WithContext(localeCtx(t, locale.LangEN))
	w := httptest.NewRecorder()
	w.Header().Set("X-Request-ID", "should-not-appear")

	Error(w, r, http.StatusNotFound)

	if strings.Contains(w.Body.String(), "should-not-appear") {
		t.Error("HTMX response must not include the request ID")
	}
}

// ── Error: full-page response ─────────────────────────────────────────────────

func TestError_FullPage_ContainsStatusCode(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/missing", nil)
	r = r.WithContext(localeCtx(t, locale.LangEN))
	w := httptest.NewRecorder()

	Error(w, r, http.StatusNotFound)

	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "<!DOCTYPE html>") {
		t.Error("full-page response must start with DOCTYPE")
	}
	if !strings.Contains(body, "HTTP 404") {
		t.Errorf("full-page missing HTTP 404, body: %q", body)
	}
}

func TestError_FullPage_IncludesRequestID_WhenPresent(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(localeCtx(t, locale.LangEN))
	w := httptest.NewRecorder()
	w.Header().Set("X-Request-ID", "req-abc-123")

	Error(w, r, http.StatusInternalServerError)

	if !strings.Contains(w.Body.String(), "req-abc-123") {
		t.Error("full-page must include X-Request-ID when set")
	}
}

func TestError_FullPage_OmitsRequestIDBlock_WhenAbsent(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(localeCtx(t, locale.LangEN))
	w := httptest.NewRecorder()

	Error(w, r, http.StatusInternalServerError)

	// The request-id <div> block should be absent when no header is set.
	if strings.Contains(w.Body.String(), "font-mono") {
		t.Error("full-page must not include request-id block when header is absent")
	}
}

// ── Thin wrappers delegate to Error with the right status ────────────────────

func TestNotFound_Returns404(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(localeCtx(t, locale.LangEN))
	w := httptest.NewRecorder()
	NotFound(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("NotFound: got %d, want 404", w.Code)
	}
}

func TestForbidden_Returns403(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(localeCtx(t, locale.LangEN))
	w := httptest.NewRecorder()
	Forbidden(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("Forbidden: got %d, want 403", w.Code)
	}
}

func TestTooManyRequests_Returns429(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(localeCtx(t, locale.LangEN))
	w := httptest.NewRecorder()
	TooManyRequests(w, r)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("TooManyRequests: got %d, want 429", w.Code)
	}
}

func TestInternalServerError_Returns500(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(localeCtx(t, locale.LangEN))
	w := httptest.NewRecorder()
	InternalServerError(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("InternalServerError: got %d, want 500", w.Code)
	}
}

// ── StatusTexts: locale key dispatch ─────────────────────────────────────────

func TestStatusTexts_KnownStatuses(t *testing.T) {
	ctx := localeCtx(t, locale.LangEN)
	statuses := []int{
		http.StatusNotFound,
		http.StatusForbidden,
		http.StatusTooManyRequests,
		http.StatusUnauthorized,
		http.StatusInternalServerError,
	}
	for _, code := range statuses {
		title, msg := StatusTexts(ctx, code)
		if title == "" {
			t.Errorf("StatusTexts(%d): empty title", code)
		}
		if msg == "" {
			t.Errorf("StatusTexts(%d): empty message", code)
		}
	}
}

func TestStatusTexts_UnknownStatusFallsBackTo500Text(t *testing.T) {
	ctx := localeCtx(t, locale.LangEN)
	title500, _ := StatusTexts(ctx, http.StatusInternalServerError)
	titleUnknown, _ := StatusTexts(ctx, 999)
	if titleUnknown != title500 {
		t.Errorf("unknown status should use 500 text: got %q, want %q", titleUnknown, title500)
	}
}

func TestStatusTexts_LocalizedForNorwegian(t *testing.T) {
	enCtx := localeCtx(t, locale.LangEN)
	nbCtx := localeCtx(t, locale.LangNB)
	enTitle, _ := StatusTexts(enCtx, http.StatusNotFound)
	nbTitle, _ := StatusTexts(nbCtx, http.StatusNotFound)
	if enTitle == nbTitle {
		t.Errorf("expected different titles for EN/NB, both returned %q", enTitle)
	}
}

// ── HomeActionLabel ───────────────────────────────────────────────────────────

func TestHomeActionLabel_ReturnsNonEmpty(t *testing.T) {
	label := HomeActionLabel(localeCtx(t, locale.LangEN))
	if label == "" {
		t.Error("HomeActionLabel: expected non-empty string")
	}
}
