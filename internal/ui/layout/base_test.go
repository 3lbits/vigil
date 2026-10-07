package layout

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"github.com/3lbits/vigil/internal/middleware"
)

func TestLayoutKeepsNavigationAndIsScoped(t *testing.T) {
	user := middleware.SessionUser{Name: "Reviewer", Role: "admin"}
	var out bytes.Buffer
	if err := Layout("Assessment", "Risk assessment", "risk", user, templ.Raw("<p>Ready</p>")).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{`ui2-shell`, `<aside aria-label="Sidebar"`, `<nav aria-label="Main navigation"`, `<main id="main-content"`, `<header class="app-page-header"`, `href="/risks"`, `action="/logout"`, `name="lang"`} {
		if !strings.Contains(html, want) {
			t.Errorf("layout missing %q", want)
		}
	}
	if strings.Index(html, `action="/logout"`) < strings.Index(html, "</nav>") {
		t.Error("account controls must be outside the main navigation")
	}
}
