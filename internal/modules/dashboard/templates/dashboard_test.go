package dashboardtemplates

import (
	"context"
	"database/sql"
	"io"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/ui/layout"
)

func renderToDoc(t *testing.T, comp interface {
	Render(context.Context, io.Writer) error
}) *goquery.Document {
	t.Helper()
	pr, pw := io.Pipe()
	go func() {
		_ = comp.Render(context.Background(), pw)
		_ = pw.Close()
	}()
	doc, err := goquery.NewDocumentFromReader(pr)
	if err != nil {
		t.Fatalf("parse rendered HTML: %v", err)
	}
	return doc
}

func TestDashboardUI2_EmptyFrameworksMessage(t *testing.T) {
	allOn := middleware.ModuleFlags{ComplianceEnabled: true, RiskEnabled: true, ActivitiesEnabled: true}
	doc := renderToDoc(t, DashboardUI2(Stats{}, nil, "all", 0, 0, 0, 0, nil, nil, nil, RiskStats{}, allOn))

	if doc.Find(`[data-testid="no-frameworks-message"]`).Length() != 1 {
		t.Error("expected no-frameworks-message when frameworks slice is nil")
	}
	if doc.Find(`[data-testid="framework-coverage-row"]`).Length() != 0 {
		t.Error("expected no framework-coverage-row elements")
	}
	if doc.Find(`.ui2-dashboard article.ui2-metric`).Length() != 4 || doc.Find(`.ui2-dashboard section.ui2-panel`).Length() == 0 {
		t.Error("dashboard must use semantic UI2 metrics and panels")
	}
}

func TestDashboardUI2_FrameworkCoverageRows(t *testing.T) {
	summaries := []FrameworkSummary{
		{Name: "ISO 27001", Coverage: 75},
		{Name: "NIST CSF", Coverage: 40},
	}
	allOn := middleware.ModuleFlags{ComplianceEnabled: true, RiskEnabled: true, ActivitiesEnabled: true}
	doc := renderToDoc(t, DashboardUI2(Stats{FrameworksCount: 2}, summaries, "all", 2, 2, 0, 0, nil, nil, nil, RiskStats{}, allOn))

	rows := doc.Find(`[data-testid="framework-coverage-row"]`)
	if rows.Length() != 2 {
		t.Errorf("expected 2 framework-coverage-row elements, got %d", rows.Length())
	}
	if doc.Find(`[data-testid="no-frameworks-message"]`).Length() != 0 {
		t.Error("should not show no-frameworks-message when frameworks exist")
	}
}

func TestDashboardUI2_RiskAndFilter(t *testing.T) {
	id := uuid.New()
	risk := RiskStats{
		Total: 1, RedCount: 1, LowMax: 5, HighMin: 12,
		Matrix:      layout.BuildRiskMatrixCells(map[[2]int]int{{3, 4}: 1}),
		TopRisks:    []db.ListTopRisksRow{{ID: id, Name: "Important risk", LikelihoodCurrent: sql.NullInt32{Int32: 3, Valid: true}, ConsequenceCurrent: sql.NullInt32{Int32: 4, Valid: true}}},
		OrgOptions:  []OrgFilterOption{{ID: id.String(), Name: "North"}},
		OrgSelected: id.String(),
	}
	flags := middleware.ModuleFlags{RiskEnabled: true, ComplianceEnabled: true}
	doc := renderToDoc(t, DashboardUI2(Stats{RequirementsCount: 10, CoveredRequirementsCount: 4}, []FrameworkSummary{{Name: "Standard", Type: "standard", Coverage: 40}}, "standard", 1, 0, 1, 0, nil, nil, nil, risk, flags))
	if got := doc.Find(".ui2-matrix-cell").Length(); got != 25 {
		t.Errorf("risk matrix cells: got %d, want 25", got)
	}
	if got := doc.Find(`.ui2-matrix-cell[data-tone="danger"]`).Length(); got == 0 {
		t.Error("expected high-risk matrix cells")
	}
	if got := doc.Find(`.ui2-table table caption`).Length(); got != 2 {
		t.Errorf("labeled tables: got %d, want 2", got)
	}
	if doc.Find(`select[name="risk_org_id"] option[selected][value="`+id.String()+`"]`).Length() != 1 ||
		doc.Find(`a[href="/?framework_type=standard"][aria-current="page"]`).Length() != 1 {
		t.Error("risk organization or framework filter selection lost")
	}
	if doc.Find(`progress[max="100"][value="40"]`).Length() != 2 {
		t.Error("expected native overall and framework progress")
	}
	if doc.Find(`.ui2-dashboard-list li a[href="/activities"]`).Length() != 0 {
		t.Error("activities link shown when module disabled")
	}
	if doc.Find(`.ui2-dashboard .ui2-grid`).Length() == 0 {
		t.Error("dashboard must use the shared UI2 grid")
	}
}
