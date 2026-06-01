// Package httputil provides HTTP handler utilities shared across modules.
package httputil

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"

	"github.com/a-h/templ"
)

// Sentinel errors for URL validation.
var (
	ErrURLInvalidScheme = errors.New("URL scheme not allowed; use http or https")
	ErrURLMissingHost   = errors.New("URL must include a host")
)

// Render writes a templ component to the response, logging any error.
func Render(w http.ResponseWriter, r *http.Request, c templ.Component) {
	if err := c.Render(r.Context(), w); err != nil {
		slog.Error("render template", "error", err)
	}
}

// RedirectWithFlash redirects to target with flash message query params.
func RedirectWithFlash(w http.ResponseWriter, r *http.Request, target, flash, flashType string) {
	http.Redirect(w, r, target+"?flash="+url.QueryEscape(flash)+"&type="+url.QueryEscape(flashType), http.StatusSeeOther)
}

// ValidateURL parses raw and returns an error if it is not a valid http or
// https URL with a non-empty host. Use this for any URL accepted from user
// input before storing or rendering it as a link.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("scheme %q: %w", u.Scheme, ErrURLInvalidScheme)
	}
	if u.Host == "" {
		return ErrURLMissingHost
	}
	return nil
}
