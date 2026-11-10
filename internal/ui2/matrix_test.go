package ui2

import (
	"bytes"
	"context"
	"fmt"
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
	if got := strings.Count(out.String(), `<span class="sr-only">0</span>`); got != 25 {
		t.Errorf("empty cells should hide zero visually but retain it for screen readers: got %d", got)
	}
}

func TestRiskLevelMatchesMatrixThresholds(t *testing.T) {
	for _, tc := range []struct {
		score int
		tone  string
	}{
		{5, "success"}, {6, "warning"}, {11, "warning"}, {12, "danger"},
	} {
		var out bytes.Buffer
		if err := RiskLevel(tc.score, 5, 12).Render(context.Background(), &out); err != nil {
			t.Fatal(err)
		}
		want := fmt.Sprintf(`<data value="%d" data-tone="%s">%d</data>`, tc.score, tc.tone, tc.score)
		if !strings.Contains(out.String(), want) {
			t.Errorf("score %d: got %q, want %q", tc.score, out.String(), want)
		}
	}
}
