package ui2

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/3lbits/vigil/internal/ui/layout"
)

func TestMatrixLabelsAxes(t *testing.T) {
	cells := layout.BuildRiskMatrixCells(nil)
	var out bytes.Buffer
	if err := Matrix("Risk map", cells, 5, 12).Render(context.Background(), &out); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`<caption class="sr-only">Risk map</caption>`, `scope="col"><span class="sr-only">`, `scope="row"><span class="sr-only">`} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("missing accessible matrix header %q", want)
		}
	}
	if got := strings.Count(out.String(), `class="ui2-matrix-cell"`); got != 25 {
		t.Errorf("matrix cells: got %d, want 25", got)
	}
}
