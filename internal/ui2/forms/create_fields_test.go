package forms

import (
	"bytes"
	"context"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/a-h/templ"
)

func TestCreateFieldsPreserveSubmittedNamesAndBindings(t *testing.T) {
	for _, tc := range []struct {
		name string
		page templ.Component
		want map[string]map[string]string
	}{
		{"measure", MeasureCreateFields(nil, "test"), map[string]map[string]string{
			"#test-measure-name":       {"name": "name", "type": "text"},
			`[name="assignee_lookup"]`: {"type": "search"},
			`[name="due_date"]`:        {"type": "date"},
			`[name="description"]`:     {},
		}},
		{"activity", ActivityCreateFields(nil, nil, "test", "Follow up"), map[string]map[string]string{
			`[name="title"]`:       {"type": "text", "value": "Follow up"},
			`[name="measure_id"]`:  {},
			`[name="assignee_id"]`: {"x-model": "assigneeID"},
			`[name="owner"]`:       {"type": "text", "x-model": "owner", ":disabled": "assigneeID !== ''"},
			`[name="recurrence"]`:  {},
			`[name="due_date"]`:    {"type": "date"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := tc.page.Render(context.Background(), &out); err != nil {
				t.Fatal(err)
			}
			doc, err := goquery.NewDocumentFromReader(&out)
			if err != nil {
				t.Fatal(err)
			}
			checkFieldAttrs(t, doc, tc.want)
		})
	}
}

func checkFieldAttrs(t *testing.T, doc *goquery.Document, want map[string]map[string]string) {
	t.Helper()
	for selector, attrs := range want {
		field := doc.Find(selector)
		if field.Length() != 1 {
			t.Errorf("expected one %s, got %d", selector, field.Length())
		}
		for key, value := range attrs {
			if got, _ := field.Attr(key); got != value {
				t.Errorf("%s %s = %q, want %q", selector, key, got, value)
			}
		}
	}
}
