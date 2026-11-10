package ui2

import (
	"bytes"
	"context"
	"html"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestFilterAndSortKeepHTMXContract(t *testing.T) {
	var out bytes.Buffer
	if err := FilterBar(FilterBarProps{ID: "filters", Action: "/activities", Target: "#results"}).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id="filters"`, `hx-target="#results"`, `change from:select`, `hx-on::after-request`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("filter missing %q", want)
		}
	}

	out.Reset()
	if err := SortTh("Name", "name", SortState{Col: "name", Dir: "asc"}, "/activities", "#results", "filters", "").Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	rendered := html.UnescapeString(out.String())
	for _, want := range []string{`scope="col"`, `aria-sort="ascending"`, `<button type="button"`, `hx-include="#filters"`, `"sort":"name","dir":"desc","offset":"0"`} {
		if !strings.Contains(rendered, want) {
			t.Errorf("sort header missing %q", want)
		}
	}
}

func TestControlsPreserveNativeAttributes(t *testing.T) {
	for _, tc := range []struct {
		name      string
		component templ.Component
		want      string
	}{
		{"link", Link("/risks", templ.Attributes{"rel": "noopener"}), `href="/risks"`},
		{"button", Button(templ.Attributes{"type": "submit", "data-tone": "danger", "@click": "toggle()"}), `@click="toggle()"`},
		{"input", Input(templ.Attributes{"type": "text", "name": "name", "required": true}), `required`},
		{"select", Select(templ.Attributes{"name": "status"}), `name="status"`},
		{"textarea", Textarea(templ.Attributes{"name": "note"}), `name="note"`},
		{"field", Field("name", "Name", true), `for="name"`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := tc.component.Render(context.Background(), &out); err != nil {
				t.Fatal(err)
			}

			if !strings.Contains(out.String(), tc.want) {
				t.Errorf("rendered %q missing %q", out.String(), tc.want)
			}
		})
	}
}

func TestUntrustedLinkSanitizesURL(t *testing.T) {
	var out bytes.Buffer
	if err := UntrustedLink("javascript:alert(1)", nil).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "javascript:") {
		t.Errorf("unsafe URL rendered: %s", out.String())
	}
}
