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
			`<p class="error-inline"><strong>HTTP %d · %s</strong> <span>%s</span></p>`,
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
<body class="error-page error-page-centered">
<main class="error-main">
<article class="error-card">
<p class="error-code">HTTP %d</p>
<h1 class="error-title">%s</h1>
<p class="error-message">%s</p>`,
		template.HTMLEscapeString(locale.LangFromContext(r.Context())),
		titleEsc, status, titleEsc, messageEsc)
	if requestID != "" {
		_, _ = fmt.Fprintf(w,
			`<p class="error-request">%s: %s</p>`,
			requestIDLabel, requestID)
	}
	_, _ = fmt.Fprintf(w, `<div class="error-actions">
<a href="/" class="error-primary">%s</a>
<button type="button" onclick="history.back()" class="error-secondary">%s</button>
</div>
</article>
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
