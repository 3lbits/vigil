package risktemplates

import (
	"bytes"
	"context"
	"database/sql"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/locale"
	"github.com/3lbits/vigil/internal/ui2"
)

func TestAssessmentDetail(t *testing.T) {
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
		CurrentMatrix:   ui2.BuildRiskMatrixCells(map[[2]int]int{{3, 4}: 1}),
		TargetMatrix:    ui2.BuildRiskMatrixCells(nil),
		LowMax:          5,
		HighMin:         12,
		CurrentUserID:   ownerID.String(),
		CanTogglePublic: true,
	}
	render := func(ctx context.Context) string {
		t.Helper()
		var out bytes.Buffer
		//nolint:contextcheck // templ components receive the context through Render.
		if err := AssessmentDetail(vm).Render(ctx, &out); err != nil {
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
	requireContains(t, empty,
		"Preview &lt;assessment&gt;", "role=\"region\"", "scope=\"row\"", `class="ui2-panel"`,
		`action="/risks/`+assessmentID.String()+`/accept"`,
		`action="/risks/`+assessmentID.String()+`/decline"`,
		`action="/risks/`+assessmentID.String()+`/delete"`,
		`href="/risks"`,
		`href="/risks/`+assessmentID.String()+`/review"`,
		`href="/risks/`+assessmentID.String()+`/step/2"`,
		"ui2-table", locale.T(ctx, "risk_none_identified"),
	)
	doc, parseErr := goquery.NewDocumentFromReader(strings.NewReader(empty))
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	if doc.Find(`form[action$="/decline"] button[type="submit"][data-tone="danger"]`).Length() != 1 ||
		doc.Find(`form[action$="/delete"] button[type="submit"][data-tone="danger"]`).Length() != 1 {
		t.Error("decline and delete forms must retain destructive submit buttons")
	}
	requireAbsent(t, empty,
		"Original view",
		`href="/risks/`+assessmentID.String()+`"`,
		`href="/risks/`+assessmentID.String()+`/ui2"`,
	)

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
	requireContains(t, populated,
		"Risk &lt;one&gt;",
		`href="/risks/`+assessmentID.String()+`/risks/`+riskID.String()+`"`,
		`<data value="12" data-tone="danger">`,
		"<span>—</span>",
	)

	vm.CurrentUserID = uuid.NewString()
	vm.CanTogglePublic = false
	other := render(ctx)
	requireAbsent(t, other,
		`action="/risks/`+assessmentID.String()+`/accept"`,
		`action="/risks/`+assessmentID.String()+`/toggle-public"`,
	)
}
