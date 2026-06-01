package httpresp

import (
	"context"
	"fmt"
	"html/template"
	"net/http"

	"github.com/3lbits/vigil/internal/locale"
)

// Error writes a localized error response for full-page and HTMX requests.
func Error(w http.ResponseWriter, r *http.Request, status int) {
	title, message := StatusTexts(r.Context(), status)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		_, _ = fmt.Fprintf(w,
			`<div class="rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-[12px] text-amber-900"><span class="font-semibold">HTTP %d · %s</span><span class="ml-1">%s</span></div>`,
			status, template.HTMLEscapeString(title), template.HTMLEscapeString(message))
		return
	}

	requestID := template.HTMLEscapeString(w.Header().Get("X-Request-ID"))
	primaryLabel := template.HTMLEscapeString(locale.T(r.Context(), "error_action_go_home"))
	titleEsc := template.HTMLEscapeString(title)
	messageEsc := template.HTMLEscapeString(message)
	backLabel := template.HTMLEscapeString(locale.T(r.Context(), "error_action_go_back"))
	requestIDLabel := template.HTMLEscapeString(locale.T(r.Context(), "error_request_id_label"))

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!DOCTYPE html>
<html lang="%s">
<head>
<meta charset="UTF-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1.0"/>
<title>%s</title>
<link rel="stylesheet" href="/public/css/output.css"/>
</head>
<body class="font-sans bg-sand-50 text-sand-900 antialiased min-h-screen flex items-center justify-center px-4">
<main class="w-full max-w-lg">
<div class="bg-white border border-sand-200 rounded-xl p-6 shadow-sm">
<div class="text-[11px] uppercase tracking-wide font-semibold text-sand-700 mb-2">HTTP %d</div>
<h1 class="text-xl font-medium mb-2">%s</h1>
<p class="text-sm text-sand-700 mb-5">%s</p>`,
		template.HTMLEscapeString(locale.LangFromContext(r.Context())),
		titleEsc, status, titleEsc, messageEsc)
	if requestID != "" {
		_, _ = fmt.Fprintf(w,
			`<div class="text-[12px] font-mono text-sand-700 bg-sand-50 border border-sand-200 rounded-md px-3 py-2 mb-5">%s: %s</div>`,
			requestIDLabel, requestID)
	}
	_, _ = fmt.Fprintf(w, `<div class="flex items-center gap-2">
<a href="/" class="inline-flex items-center text-[12px] py-1.5 px-3 rounded-md bg-sand-900 text-white hover:bg-sand-800 transition-colors">%s</a>
<button type="button" onclick="history.back()" class="inline-flex items-center text-[12px] py-1.5 px-3 rounded-md border border-sand-300 text-sand-700 hover:bg-sand-50 transition-colors cursor-pointer">%s</button>
</div>
</div>
</main>
</body>
</html>`, primaryLabel, backLabel)
}

func NotFound(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusNotFound)
}

func Forbidden(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusForbidden)
}

func TooManyRequests(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusTooManyRequests)
}

func InternalServerError(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusInternalServerError)
}

// StatusTexts returns localized title/message for a standard HTTP status.
func StatusTexts(ctx context.Context, status int) (string, string) {
	switch status {
	case http.StatusNotFound:
		return locale.T(ctx, "error_404_title"), locale.T(ctx, "error_404_message")
	case http.StatusForbidden:
		return locale.T(ctx, "error_403_title"), locale.T(ctx, "error_403_message")
	case http.StatusTooManyRequests:
		return locale.T(ctx, "error_429_title"), locale.T(ctx, "error_429_message")
	case http.StatusUnauthorized:
		return locale.T(ctx, "error_401_title"), locale.T(ctx, "error_401_message")
	default:
		return locale.T(ctx, "error_500_title"), locale.T(ctx, "error_500_message")
	}
}

// HomeActionLabel returns localized primary action label for error pages.
func HomeActionLabel(ctx context.Context) string {
	return locale.T(ctx, "error_action_go_home")
}
