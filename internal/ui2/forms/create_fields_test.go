package forms

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
)

func TestCreateFieldsPreserveSubmittedNamesAndBindings(t *testing.T) {
	for _, tc := range []struct {
		name string
		page templ.Component
		want []string
	}{
		{"measure", MeasureCreateFields(nil, "test"), []string{
			`id="test-measure-name" name="name" type="text"`, `name="assignee_lookup"`,
			`name="due_date" type="date"`, `name="description"`,
		}},
		{"activity", ActivityCreateFields(nil, nil, "test", "Follow up"), []string{
			`name="title" type="text" value="Follow up"`, `name="measure_id"`,
			`name="assignee_id" x-model="assigneeID"`, `name="owner" type="text"`,
			`name="recurrence"`, `name="due_date" type="date"`,
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := tc.page.Render(context.Background(), &out); err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q", want)
				}
			}
		})
	}
}
