package dashboardtemplates

import (
	"context"
	"io"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/3lbits/vigil/internal/middleware"
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

func TestDashboardContent_EmptyFrameworksMessage(t *testing.T) {
	allOn := middleware.ModuleFlags{ComplianceEnabled: true, RiskEnabled: true, ActivitiesEnabled: true}
	doc := renderToDoc(t, DashboardContent(Stats{}, nil, "all", 0, 0, 0, 0, nil, nil, nil, RiskStats{}, allOn))

	if doc.Find(`[data-testid="no-frameworks-message"]`).Length() != 1 {
		t.Error("expected no-frameworks-message when frameworks slice is nil")
	}
	if doc.Find(`[data-testid="framework-coverage-row"]`).Length() != 0 {
		t.Error("expected no framework-coverage-row elements")
	}
}

func TestDashboardContent_FrameworkCoverageRows(t *testing.T) {
	summaries := []FrameworkSummary{
		{Name: "ISO 27001", Coverage: 75},
		{Name: "NIST CSF", Coverage: 40},
	}
	allOn := middleware.ModuleFlags{ComplianceEnabled: true, RiskEnabled: true, ActivitiesEnabled: true}
	doc := renderToDoc(t, DashboardContent(Stats{FrameworksCount: 2}, summaries, "all", 2, 2, 0, 0, nil, nil, nil, RiskStats{}, allOn))

	rows := doc.Find(`[data-testid="framework-coverage-row"]`)
	if rows.Length() != 2 {
		t.Errorf("expected 2 framework-coverage-row elements, got %d", rows.Length())
	}
	if doc.Find(`[data-testid="no-frameworks-message"]`).Length() != 0 {
		t.Error("should not show no-frameworks-message when frameworks exist")
	}
}

func TestStats_ImplementedPct(t *testing.T) {
	tests := []struct {
		measures    int64
		implemented int64
		expectedPct int
	}{
		{measures: 0, implemented: 0, expectedPct: 0},
		{measures: 10, implemented: 5, expectedPct: 50},
		{measures: 3, implemented: 3, expectedPct: 100},
		{measures: 4, implemented: 1, expectedPct: 25},
	}
	for _, tc := range tests {
		s := Stats{MeasuresCount: tc.measures, ImplementedCount: tc.implemented}
		if got := s.ImplementedPct(); got != tc.expectedPct {
			t.Errorf("ImplementedPct(%d/%d): expected %d, got %d", tc.implemented, tc.measures, tc.expectedPct, got)
		}
	}
}
