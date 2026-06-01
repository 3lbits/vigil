// Package about serves the in-app project information page.
package about

import (
	"net/http"

	"github.com/3lbits/vigil/internal/httputil"
	"github.com/3lbits/vigil/internal/middleware"
	abouttemplates "github.com/3lbits/vigil/internal/modules/about/templates"
	"github.com/3lbits/vigil/internal/ui/layout"
)

type Handler struct{}

func NewHandler() *Handler { return &Handler{} }

func (h *Handler) Show(w http.ResponseWriter, r *http.Request) {
	user, _ := middleware.FromContext(r.Context())
	httputil.Render(w, r, layout.Layout("nav_about", "about_subtitle", "about", user, abouttemplates.About()))
}
