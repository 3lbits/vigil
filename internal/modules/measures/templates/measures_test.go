package measurestemplates

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/google/uuid"
	"github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/3lbits/vigil/internal/db"
	"github.com/3lbits/vigil/internal/locale"
)

func testCtx(t *testing.T) context.Context {
	t.Helper()
	bundle, err := locale.NewBundle()
	if err != nil {
		t.Fatalf("locale bundle: %v", err)
	}
	l := i18n.NewLocalizer(bundle, locale.DefaultLang)
	return locale.SetLocalizer(context.Background(), l, locale.DefaultLang)
}

func renderToDoc(t *testing.T, comp interface {
	Render(context.Context, io.Writer) error
}) *goquery.Document {
	t.Helper()
	ctx := testCtx(t)
	pr, pw := io.Pipe()
	go func() {
		_ = comp.Render(ctx, pw)
		_ = pw.Close()
	}()
	doc, err := goquery.NewDocumentFromReader(pr)
	if err != nil {
		t.Fatalf("parse rendered HTML: %v", err)
	}
	return doc
}

// ── MeasureList ───────────────────────────────────────────────────────────────

func TestMeasureList_ShowsCount(t *testing.T) {
	measures := []MeasureVM{
		{Measure: db.Measure{ID: uuid.New(), Name: "Firewall rules", Status: "implemented"}},
		{Measure: db.Measure{ID: uuid.New(), Name: "MFA enforcement", Status: "planned"}},
	}
	doc := renderToDoc(t, MeasureList(measures, "", "", false, "", "", false, 50))

	el := doc.Find(`[data-testid="measure-count"]`)
	if el.Length() != 1 {
		t.Fatal("expected measure-count element")
	}
	if !strings.Contains(el.Text(), "2") {
		t.Errorf("expected '2' in count, got %q", el.Text())
	}
}

// ── MeasuresTable ─────────────────────────────────────────────────────────────

func TestMeasuresTable_EmptyState(t *testing.T) {
	doc := renderToDoc(t, MeasuresTable(nil, false, 50, "", "", false))

	if doc.Find(`[data-testid="no-measures-empty"]`).Length() != 1 {
		t.Error("expected no-measures-empty when list is empty")
	}
}

func TestMeasuresTable_RendersRows(t *testing.T) {
	measures := []MeasureVM{
		{Measure: db.Measure{ID: uuid.New(), Name: "Encryption at rest", Status: "implemented"}},
		{Measure: db.Measure{ID: uuid.New(), Name: "Key rotation", Status: "planned"}},
	}
	doc := renderToDoc(t, MeasuresTable(measures, false, 50, "", "", false))

	if doc.Find(`[data-testid="no-measures-empty"]`).Length() != 0 {
		t.Error("should not show empty state when measures exist")
	}
	rows := doc.Find("tbody tr")
	if rows.Length() != 2 {
		t.Errorf("expected 2 table rows, got %d", rows.Length())
	}
}

func TestMeasuresTable_RendersAllPassedRows(t *testing.T) {
	// Filtering is server-side; the template renders whatever rows it receives.
	measures := []MeasureVM{
		{Measure: db.Measure{ID: uuid.New(), Name: "Implemented measure", Status: "implemented"}},
		{Measure: db.Measure{ID: uuid.New(), Name: "Planned measure", Status: "planned"}},
	}
	doc := renderToDoc(t, MeasuresTable(measures, false, 50, "implemented", "", false))

	rows := doc.Find("tbody tr")
	if rows.Length() != 2 {
		t.Errorf("template should render all received rows (filtering is DB-side), got %d", rows.Length())
	}
}

func TestMeasuresTable_LoadMoreRow(t *testing.T) {
	measures := []MeasureVM{
		{Measure: db.Measure{ID: uuid.New(), Name: "Measure 1", Status: "implemented"}},
	}
	doc := renderToDoc(t, MeasuresTable(measures, true, 50, "", "", false))

	rows := doc.Find("tbody tr")
	if rows.Length() != 2 {
		t.Errorf("expected 1 data row + 1 load-more row, got %d", rows.Length())
	}
}
