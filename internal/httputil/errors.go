package httputil

import (
	"net/http"

	"github.com/3lbits/vigil/internal/httpresp"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/ui/layout"
)

// Error writes a localized error response for full-page and HTMX requests.
func Error(w http.ResponseWriter, r *http.Request, status int) {
	title, message := httpresp.StatusTexts(r.Context(), status)
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(status)
		Render(w, r, ErrorInline(status, title, message))
		return
	}

	requestID := w.Header().Get("X-Request-ID")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	data := ErrorPageData{
		Status:       status,
		Title:        title,
		Message:      message,
		RequestID:    requestID,
		PrimaryHref:  "/",
		PrimaryLabel: httpresp.HomeActionLabel(r.Context()),
	}
	if user, ok := middleware.FromContext(r.Context()); ok {
		Render(w, r, layout.Layout("error_page_title", "error_page_subtitle", "", user, ErrorPageCard(data)))
		return
	}
	Render(w, r, ErrorPage(data))
}

func NotFound(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusNotFound)
}

func Forbidden(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusForbidden)
}

func InternalServerError(w http.ResponseWriter, r *http.Request) {
	Error(w, r, http.StatusInternalServerError)
}
