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
	doc := renderToDoc(t, MeasureList(measures, "", "", "name", "asc", false, "", "", false, 50))

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
	doc := renderToDoc(t, MeasuresTable(nil, false, 50, "", "", "name", "asc", false))

	if doc.Find(`[data-testid="no-measures-empty"]`).Length() != 1 {
		t.Error("expected no-measures-empty when list is empty")
	}
}

func TestMeasuresTable_RendersRows(t *testing.T) {
	measures := []MeasureVM{
		{Measure: db.Measure{ID: uuid.New(), Name: "Encryption at rest", Status: "implemented"}},
		{Measure: db.Measure{ID: uuid.New(), Name: "Key rotation", Status: "planned"}},
	}
	doc := renderToDoc(t, MeasuresTable(measures, false, 50, "", "", "name", "asc", false))

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
	doc := renderToDoc(t, MeasuresTable(measures, false, 50, "implemented", "", "name", "asc", false))

	rows := doc.Find("tbody tr")
	if rows.Length() != 2 {
		t.Errorf("template should render all received rows (filtering is DB-side), got %d", rows.Length())
	}
}

func TestMeasuresTable_LoadMoreRow(t *testing.T) {
	measures := []MeasureVM{
		{Measure: db.Measure{ID: uuid.New(), Name: "Measure 1", Status: "implemented"}},
	}
	doc := renderToDoc(t, MeasuresTable(measures, true, 50, "", "", "name", "asc", false))

	rows := doc.Find("tbody tr")
	if rows.Length() != 2 {
		t.Errorf("expected 1 data row + 1 load-more row, got %d", rows.Length())
	}
}

// ── measureLinksSection (link-search input) ────────────────────────────────────

// TestMeasureLinksSection_RequirementSearchHasHxGet guards two regressions in the
// requirement link-search input:
//   - hx-get must be present and non-empty. Passing the URL as templ.SafeURL inside
//     an Attrs map silently drops it (templ.RenderAttributes type-switches on the
//     exact dynamic type and has no SafeURL case), so htmx never fired a request.
//   - the live-search "text-[13px]" class must survive. It used to be passed via
//     Attrs["class"], producing a duplicate class attribute the HTML parser discards;
//     it now goes through InputProps.Class so it merges into the single class attr.
func TestMeasureLinksSection_RequirementSearchHasHxGet(t *testing.T) {
	id := uuid.New()
	doc := renderToDoc(t, measureLinksSection(MeasureEditVM{Measure: db.Measure{ID: id}}))

	input := doc.Find("#requirement-link-search")
	if input.Length() != 1 {
		t.Fatalf("expected requirement-link-search input, found %d", input.Length())
	}
	hxGet, ok := input.Attr("hx-get")
	if !ok || hxGet == "" {
		t.Fatalf("hx-get missing or empty (SafeURL-in-Attrs regression): %q", hxGet)
	}
	if !strings.Contains(hxGet, "/measures/"+id.String()+"/requirements/search") {
		t.Errorf("unexpected hx-get target: %q", hxGet)
	}
	if class, _ := input.Attr("class"); !strings.Contains(class, "text-[13px]") {
		t.Errorf("text-[13px] class dropped (duplicate-class regression): %q", class)
	}
}
