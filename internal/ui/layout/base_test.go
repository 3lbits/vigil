package layout

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
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

func TestNavigationIconsAreDistinctAndInheritColor(t *testing.T) {
	var out bytes.Buffer
	if err := Layout("Overview", "", "dashboard", middleware.SessionUser{Role: "admin"}, templ.Raw("")).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	doc, err := goquery.NewDocumentFromReader(&out)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]string{}
	for _, href := range []string{"/risks", "/risk-register", "/avvik", "/me", "/admin"} {
		icon := doc.Find(`.sidebar-nav-link[href="` + href + `"] svg`)
		if icon.Length() != 1 || icon.AttrOr("aria-hidden", "") != "true" || icon.AttrOr("width", "") != "18" {
			t.Errorf("%s must have one decorative 18px icon", href)
			continue
		}
		path := icon.Find("path")
		if path.Length() != 1 || path.AttrOr("fill", "") != "currentColor" {
			t.Errorf("%s icon must inherit its color", href)
			continue
		}
		shape := path.AttrOr("d", "")
		if shape == "" || seen[shape] != "" {
			t.Errorf("%s must have a distinct icon from %s", href, seen[shape])
		}
		seen[shape] = href
	}
}
