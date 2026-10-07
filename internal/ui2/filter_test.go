package ui2

import (
	"bytes"
	"context"
	"html"
	"strings"
	"testing"
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
