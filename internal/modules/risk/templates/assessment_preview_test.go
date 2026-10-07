package risktemplates

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/locale"
	"github.com/3lbits/vigil/internal/ui/layout"
)

func TestAssessmentDetailPreview(t *testing.T) {
	bundle, err := locale.NewBundle()
	if err != nil {
		t.Fatal(err)
	}
	ctx := locale.SetLocalizer(context.Background(), i18n.NewLocalizer(bundle, locale.LangEN), locale.LangEN)
	assessmentID := uuid.New()
	riskID := uuid.New()
	ownerID := uuid.New()
	vm := AssessmentDetailVM{
		Assessment: db.RiskAssessment{
			ID:          assessmentID,
			Name:        "Preview <assessment>",
			Status:      "pending_acceptance",
			RiskOwnerID: uuid.NullUUID{UUID: ownerID, Valid: true},
		},
		CurrentMatrix:   layout.BuildRiskMatrixCellsT(map[[2]int]int{{3, 4}: 1}, layout.RiskThresholds{LowMax: 5, HighMin: 12}),
		TargetMatrix:    layout.BuildRiskMatrixCellsT(nil, layout.RiskThresholds{LowMax: 5, HighMin: 12}),
		LowMax:          5,
		HighMin:         12,
		CurrentUserID:   ownerID.String(),
		CanTogglePublic: true,
	}
	render := func(ctx context.Context) string {
		t.Helper()
		var out bytes.Buffer
		//nolint:contextcheck // templ components receive the context through Render.
		if err := AssessmentDetailPreview(vm).Render(ctx, &out); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}
	empty := render(ctx)
	if strings.Contains(empty, "NS 5814") ||
		!strings.Contains(empty, locale.T(ctx, "label_with_existing_controls")) ||
		!strings.Contains(empty, locale.T(ctx, "label_after_planned_treatment")) {
		t.Fatal("matrix subtitles must describe controls and treatment without NS 5814")
	}
	for _, want := range []string{
		"Preview &lt;assessment&gt;", "role=\"region\"", "scope=\"row\"",
		`action="/risks/` + assessmentID.String() + `/accept"`,
		`action="/risks/` + assessmentID.String() + `/decline"`,
		`action="/risks/` + assessmentID.String() + `/delete"`,
		`href="/risks/` + assessmentID.String() + `"`,
		"ui2-table", locale.T(ctx, "risk_none_identified"),
	} {
		if !strings.Contains(empty, want) {
			t.Errorf("empty preview missing %q", want)
		}
	}

	vm.Risks = []db.Risk{{
		ID:                 riskID,
		Name:               "Risk <one>",
		LikelihoodCurrent:  sql.NullInt32{Int32: 3, Valid: true},
		ConsequenceCurrent: sql.NullInt32{Int32: 4, Valid: true},
		RiskDecision:       "treat",
	}}
	populated := render(ctx)
	if got := strings.Count(populated, `class="ui2-matrix-cell"`); got != 50 {
		t.Errorf("matrix cells: got %d, want 50", got)
	}
	for _, want := range []string{
		"Risk &lt;one&gt;",
		`href="/risks/` + assessmentID.String() + `/risks/` + riskID.String() + `"`,
		`<data value="12" data-tone="danger">`,
		"<span>—</span>",
	} {
		if !strings.Contains(populated, want) {
			t.Errorf("populated preview missing %q", want)
		}
	}

	vm.CurrentUserID = uuid.NewString()
	vm.CanTogglePublic = false
	other := render(ctx)
	if strings.Contains(other, `action="/risks/`+assessmentID.String()+`/accept"`) ||
		strings.Contains(other, `action="/risks/`+assessmentID.String()+`/toggle-public"`) {
		t.Fatal("non-owner preview includes owner-only actions")
	}
}

func TestPreviewToneThresholds(t *testing.T) {
	for _, tc := range []struct {
		score int
		want  string
	}{
		{5, "success"}, {6, "warning"}, {11, "warning"}, {12, "danger"},
	} {
		if got := previewTone(tc.score, 5, 12); got != tc.want {
			t.Errorf("score %d: got %q, want %q", tc.score, got, tc.want)
		}
	}
}
