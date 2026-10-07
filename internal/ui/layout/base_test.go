package layout

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/ui"
)

func TestLayoutPreviewKeepsNavigationAndIsScoped(t *testing.T) {
	user := middleware.SessionUser{Name: "Reviewer", Role: "admin"}
	var out bytes.Buffer
	if err := LayoutPreview("Assessment", "Risk assessment", "risk", user, ui.Flash("Ready", "success")).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	html := out.String()
	for _, want := range []string{`ui2-shell`, `aria-label="Main navigation"`, `id="main-content"`, `href="/risks"`, `action="/logout"`, `name="lang"`} {
		if !strings.Contains(html, want) {
			t.Errorf("preview layout missing %q", want)
		}
	}
	out.Reset()
	if err := Layout("Assessment", "Risk assessment", "risk", user, ui.Flash("Ready", "success")).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "ui2-shell") || !strings.Contains(out.String(), `aria-label="Main navigation"`) {
		t.Fatal("existing layout changed or lost navigation")
	}
}
