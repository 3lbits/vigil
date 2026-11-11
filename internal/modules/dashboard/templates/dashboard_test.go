package dashboardtemplates

import (
	"context"
	"database/sql"
	"io"
	"testing"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/middleware"
	"github.com/3lbits/vigil/internal/ui2"
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
	if doc.Find(`.ui2-dashboard article.ui2-metric`).Length() != 3 || doc.Find(`.ui2-dashboard section.ui2-panel`).Length() == 0 {
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
	if rows.Eq(0).Find(`meter[value="75"][aria-label="ISO 27001"]`).Length() != 1 ||
		rows.Eq(1).Find(`meter[value="40"][aria-label="NIST CSF"]`).Length() != 1 ||
		rows.Eq(0).Find(`data[value="75"]`).Length() != 1 ||
		rows.Eq(1).Find(`data[value="40"]`).Length() != 1 {
		t.Error("framework rows must show labeled native coverage bars and percentages")
	}
	if rows.Eq(0).Find(`data`).Text() != "75 %" {
		t.Error("coverage percentage must remain visible in the compact row")
	}
}

func TestDashboardUI2_RiskAndFilter(t *testing.T) {
	id := uuid.New()
	risk := RiskStats{
		Total: 1, RedCount: 1, LowMax: 5, HighMin: 12,
		Matrix:      ui2.BuildRiskMatrixCells(map[[2]int]int{{3, 4}: 1}),
		TopRisks:    []db.ListTopRisksRow{{ID: id, AssessmentID: id, Name: "Assessment: Important risk", AssessmentName: "Assessment", LikelihoodCurrent: sql.NullInt32{Int32: 3, Valid: true}, ConsequenceCurrent: sql.NullInt32{Int32: 4, Valid: true}}},
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
	if got := doc.Find(`.ui2-table table caption`).Length(); got != 1 {
		t.Errorf("labeled matrix tables: got %d, want 1", got)
	}
	if doc.Find(`select[name="risk_org_id"] option[selected][value="`+id.String()+`"]`).Length() != 1 ||
		doc.Find(`a[href="/?framework_type=standard"][aria-current="page"]`).Length() != 1 {
		t.Error("risk organization or framework filter selection lost")
	}
	if doc.Find(`.ui2-dashboard-metrics data[value="4"]`).Length() != 1 ||
		doc.Find(`[data-testid="framework-coverage-row"] meter[value="40"]`).Length() != 1 {
		t.Error("coverage count and framework bar must remain visible")
	}
	if doc.Find(`.ui2-dashboard-list li a[href="/risks/`+id.String()+`/risks/`+id.String()+`"]`).Length() != 1 {
		t.Error("top risk must link to its assessment")
	}
	if doc.Find(`.ui2-dashboard-list li a[href="/risks/`+id.String()+`/risks/`+id.String()+`"]`).Text() != "Important risk" ||
		doc.Find(".ui2-dashboard-risk-score .ui2-badge[data-tone=warning]").Length() != 1 {
		t.Error("risk name should not repeat assessment, and missing decision must be visible")
	}
	if doc.Find(`.ui2-dashboard-list li a[href="/activities"]`).Length() != 0 {
		t.Error("activities link shown when module disabled")
	}
	if doc.Find(`.ui2-dashboard .ui2-grid`).Length() == 0 {
		t.Error("dashboard must use the shared UI2 grid")
	}
}

func TestDashboardUI2_PrioritizesAssignedWorkAndGroupsRisk(t *testing.T) {
	id := uuid.New()
	risk := RiskStats{
		Total:    1,
		Matrix:   ui2.BuildRiskMatrixCells(map[[2]int]int{{3, 4}: 1}),
		TopRisks: []db.ListTopRisksRow{{ID: id, AssessmentID: id, Name: "Important risk"}},
	}
	doc := renderToDoc(t, DashboardUI2(
		Stats{}, nil, "all", 0, 0, 0, 0, nil,
		[]db.ListActivitiesForUserRow{{ID: id, Title: "My activity"}},
		[]db.Measure{{ID: id, Name: "My measure"}},
		risk, middleware.ModuleFlags{ActivitiesEnabled: true, RiskEnabled: true, ComplianceEnabled: true},
	))

	grids := doc.Find(".ui2-dashboard > .ui2-grid")
	if grids.First().Find(`a[href="/activities/`+id.String()+`"], a[href="/measures/`+id.String()+`"]`).Length() != 2 {
		t.Error("assigned work must appear before the risk overview")
	}
	if grids.Eq(1).Find(`.ui2-matrix, a[href="/risks/`+id.String()+`/risks/`+id.String()+`"]`).Length() != 2 {
		t.Error("risk map and top risks must appear together")
	}
	if doc.Find(".ui2-dashboard > .ui2-panel [data-testid=no-frameworks-message]").Length() != 1 {
		t.Error("frameworks must have their own full-width panel")
	}
}

func TestDashboardUI2_OverdueActivitiesStandOut(t *testing.T) {
	id := uuid.New()
	doc := renderToDoc(t, DashboardUI2(
		Stats{OverdueActivitiesCount: 1}, nil, "all", 0, 0, 0, 0,
		[]db.ListRecentActivitiesRow{{ID: id, Status: "overdue"}},
		[]db.ListActivitiesForUserRow{{ID: id, Status: "overdue"}},
		nil, RiskStats{}, middleware.ModuleFlags{ActivitiesEnabled: true},
	))

	if !doc.Find(".ui2-dashboard > :first-child").Is(".ui2-panel") ||
		doc.Find(".ui2-dashboard-actions .ui2-notice[data-tone=danger]").Length() != 1 {
		t.Error("overdue work must lead with an actionable danger notice")
	}
	links := doc.Find(`a[href="/activities/` + id.String() + `"]`)
	if links.Length() != 2 {
		t.Fatalf("expected activity in personal and recent lists, got %d links", links.Length())
	}
	links.Each(func(_ int, link *goquery.Selection) {
		if link.Closest("li").Find(".ui2-badge[data-tone=danger]").Length() != 1 {
			t.Error("overdue activity must be marked in both lists")
		}
	})
	if doc.Find(".ui2-dashboard > .ui2-panel details:not([open]) > summary").Length() != 1 {
		t.Error("recent team activity must be available without crowding the overview")
	}
}

func TestDashboardUI2_RecentActivityShowsUpdatedDate(t *testing.T) {
	doc := renderToDoc(t, DashboardUI2(
		Stats{}, nil, "all", 0, 0, 0, 0,
		[]db.ListRecentActivitiesRow{{UpdatedAt: time.Date(2026, 11, 10, 0, 0, 0, 0, time.UTC), DueDate: sql.NullTime{Time: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), Valid: true}}},
		nil, nil, RiskStats{}, middleware.ModuleFlags{ActivitiesEnabled: true},
	))
	if doc.Find(`details time[datetime="2026-11-10"]`).Length() != 1 {
		t.Error("recent activity date must match its update-time ordering")
	}
}

func TestDashboardUI2_FilterWithoutMatches(t *testing.T) {
	doc := renderToDoc(t, DashboardUI2(
		Stats{}, nil, "standard", 2, 2, 0, 0, nil, nil, nil, RiskStats{},
		middleware.ModuleFlags{ComplianceEnabled: true},
	))
	if doc.Find(`a[href="/?framework_type=standard"][aria-current=page]`).Length() != 1 ||
		doc.Find(`a[href="/?framework_type=directive"]`).Length() != 0 ||
		doc.Find(`[data-testid=no-frameworks-message] a[href="/"]`).Length() != 1 {
		t.Error("empty selected filter must be visible with a way back to all frameworks")
	}
}

func TestDashboardUI2_NoticesHaveSeparateMessagesAndActions(t *testing.T) {
	doc := renderToDoc(t, DashboardUI2(
		Stats{OverdueActivitiesCount: 2},
		[]FrameworkSummary{{Name: "Uncovered"}}, "all", 1, 1, 0, 0,
		nil, nil, nil, RiskStats{},
		middleware.ModuleFlags{ActivitiesEnabled: true, ComplianceEnabled: true},
	))

	for _, notice := range []struct {
		tone, href string
	}{
		{"danger", "/activities?filter=overdue"},
		{"warning", "/compliance"},
	} {
		message := doc.Find(`.ui2-dashboard-actions .ui2-notice[data-tone="` + notice.tone + `"]`)
		if message.Children().Length() != 3 ||
			!message.Children().Eq(0).Is("strong") ||
			!message.Children().Eq(1).Is("span") ||
			message.Find(`a[href="`+notice.href+`"]`).Length() != 1 {
			t.Errorf("%s notice needs a distinct summary, description, and action", notice.tone)
		}
	}
}
