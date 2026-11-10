package measurestemplates

import (
	"context"
	"database/sql"
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
	if doc.Find(`.ui2-table[role="region"][tabindex="0"] table caption.sr-only`).Length() != 1 {
		t.Error("measures table must use the accessible UI2 table component")
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
//   - the live-search ops-input class must survive in a single class attribute.
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
	if !input.HasClass("ops-input") {
		t.Errorf("ops-input class dropped: %q", input.AttrOr("class", ""))
	}
	classAttrs := 0
	for _, attr := range input.Get(0).Attr {
		if attr.Key == "class" {
			classAttrs++
		}
	}
	if classAttrs != 1 {
		t.Errorf("expected one class attribute (duplicate-class regression), got %d", classAttrs)
	}
}

func TestMeasureLinksSectionSanitizesStoredURL(t *testing.T) {
	doc := renderToDoc(t, measureDocSection(MeasureEditVM{
		Links: []db.MeasureLink{{Url: "javascript:alert(1)"}, {Url: "https://example.org/doc"}},
	}))
	if doc.Find(`a[href^="javascript:"]`).Length() != 0 {
		t.Error("stored documentation URL bypassed templ sanitization")
	}
	if doc.Find(`a[href="https://example.org/doc"]`).Length() != 1 {
		t.Error("valid documentation URL was lost")
	}
}

func TestMeasureDetailUsesConfiguredRiskThresholds(t *testing.T) {
	risk := db.Risk{
		Name:               "Risk",
		LikelihoodCurrent:  sql.NullInt32{Int32: 1, Valid: true},
		ConsequenceCurrent: sql.NullInt32{Int32: 5, Valid: true},
		LikelihoodTarget:   sql.NullInt32{Int32: 2, Valid: true},
		ConsequenceTarget:  sql.NullInt32{Int32: 5, Valid: true},
	}
	doc := renderToDoc(t, MeasureDetail(MeasureDetailVM{
		LinkedRisks: []db.Risk{risk}, LowMax: 4, HighMin: 10,
	}))
	if doc.Find(`.ops-risk-score[data-tone="warning"]`).Length() != 1 ||
		doc.Find(`.ops-risk-score[data-tone="danger"]`).Length() != 1 {
		t.Error("linked risk scores do not use configured thresholds")
	}
}

func TestMeasureForm_KeepsFieldsAndErrorAlert(t *testing.T) {
	id := uuid.New()
	doc := renderToDoc(t, MeasureForm(&db.Measure{ID: id, Name: "MFA", Status: "planned"}, nil, "Save failed", "error"))
	if doc.Find(`form[action="/measures/`+id.String()+`"] input[name="_csrf"]`).Length() != 1 {
		t.Error("measure form must retain its action and CSRF field")
	}
	if doc.Find(`input[name="name"][value="MFA"]`).Length() != 1 || doc.Find(`select[name="status"] option[value="planned"][selected]`).Length() != 1 {
		t.Error("measure edit fields lost their values")
	}
	if doc.Find(`button[formaction="/measures/`+id.String()+`/delete"][formmethod="POST"]`).Length() != 1 {
		t.Error("delete button lost its form override")
	}
	if doc.Find(`[role="alert"][data-tone="danger"]`).Text() != "Save failed" {
		t.Error("error flash must remain an alert")
	}
}
