package compliancetemplates

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

// ── FrameworkList ─────────────────────────────────────────────────────────────

func TestFrameworkList_ShowsCount(t *testing.T) {
	fws := []FrameworkVM{
		{Framework: db.Framework{ID: uuid.New(), Name: "ISO 27001"}, Coverage: 80, TotalReqs: 10},
		{Framework: db.Framework{ID: uuid.New(), Name: "NIST CSF"}, Coverage: 40, TotalReqs: 5},
	}
	doc := renderToDoc(t, FrameworkList(fws, "", "", ""))

	el := doc.Find(`[data-testid="framework-count"]`)
	if el.Length() != 1 {
		t.Fatal("expected framework-count element")
	}
	if !strings.Contains(el.Text(), "2") {
		t.Errorf("expected count to contain '2', got %q", el.Text())
	}
}

func TestFrameworkList_EmptyState(t *testing.T) {
	doc := renderToDoc(t, FrameworkList(nil, "", "", ""))

	if doc.Find(`[data-testid="no-frameworks-empty"]`).Length() != 1 {
		t.Error("expected no-frameworks-empty element when list is empty")
	}
	if doc.Find(`[data-testid="framework-count"]`).Length() != 1 {
		t.Error("expected framework-count element even in empty state")
	}
	count := doc.Find(`[data-testid="framework-count"]`).Text()
	if !strings.Contains(count, "0") {
		t.Errorf("expected '0' in count text, got %q", count)
	}
}

func TestFrameworkDetail_ShowsSOAColumnsAndReferenceValue(t *testing.T) {
	fwID := uuid.New()
	doc := renderToDoc(t, FrameworkDetail(FrameworkDetailVM{
		Framework: db.Framework{ID: fwID, Name: "ISO 27001", ShortName: "ISO"},
		Requirements: []db.Requirement{
			{
				ID:                uuid.New(),
				FrameworkID:       fwID,
				Ref:               "A.5.1",
				Title:             "Policies for information security",
				NotRelevant:       true,
				NotRelevantReason: "Out of scope for this unit",
			},
		},
		CanEdit: true,
	}))

	if !strings.Contains(doc.Text(), "Applicability") {
		t.Fatal("expected Applicability column in framework detail")
	}
	if !strings.Contains(doc.Text(), "Justification") {
		t.Fatal("expected Justification column in framework detail")
	}
	if !strings.Contains(doc.Text(), "A.5.1") {
		t.Fatal("expected actual requirement ref value to be rendered")
	}
	if strings.Contains(doc.Text(), "req.Ref") {
		t.Fatal("did not expect literal req.Ref text in rendered output")
	}
}

func TestRequirementDetail_ShowsRelevantStatusWhenApplicable(t *testing.T) {
	doc := renderToDoc(t, RequirementDetail(RequirementDetailVM{
		Requirement: db.Requirement{
			ID:          uuid.New(),
			FrameworkID: uuid.New(),
			Ref:         "A.5.1",
			Title:       "Policies for information security",
			NotRelevant: false,
		},
		FrameworkName:  "ISO 27001",
		FrameworkShort: "ISO",
	}))

	if !strings.Contains(doc.Text(), "Status") {
		t.Fatal("expected status label in requirement detail")
	}
	if !strings.Contains(doc.Text(), "Relevant") {
		t.Fatal("expected relevant status badge in requirement detail")
	}
}

func TestFrameworkDetail_ShowsImplementationMappingWithMeasures(t *testing.T) {
	fwID := uuid.New()
	reqID := uuid.New()
	doc := renderToDoc(t, FrameworkDetail(FrameworkDetailVM{
		Framework: db.Framework{ID: fwID, Name: "ISO 27001", ShortName: "ISO"},
		Requirements: []db.Requirement{
			{
				ID:          reqID,
				FrameworkID: fwID,
				Ref:         "A.5.2",
				Title:       "Information security roles",
			},
		},
		RequirementImplementations: []RequirementVM{
			{
				Requirement: db.Requirement{
					ID:          reqID,
					FrameworkID: fwID,
					Ref:         "A.5.2",
					Title:       "Information security roles",
				},
				Measures: []db.ListMeasuresForRequirementRow{
					{ID: uuid.New(), Name: "Security policy review", Status: "implemented"},
				},
			},
		},
	}))

	if !strings.Contains(doc.Text(), "Implementation mapping (requirements") {
		t.Fatal("expected implementation mapping section")
	}
	if !strings.Contains(doc.Text(), "Measures") {
		t.Fatal("expected measures column in implementation mapping table")
	}
	if !strings.Contains(doc.Text(), "Security policy review") {
		t.Fatal("expected linked measure name in implementation mapping table")
	}
}

func TestFrameworkDetail_ShowsQuickLinksToSections(t *testing.T) {
	fwID := uuid.New()
	doc := renderToDoc(t, FrameworkDetail(FrameworkDetailVM{
		Framework: db.Framework{ID: fwID, Name: "ISO 27001", ShortName: "ISO"},
		Requirements: []db.Requirement{
			{
				ID:          uuid.New(),
				FrameworkID: fwID,
				Ref:         "A.5.1",
				Title:       "Policies for information security",
			},
		},
	}))

	if doc.Find(`a[href="#implementation-mapping"]`).Length() != 1 {
		t.Fatal("expected quick link to implementation mapping section")
	}
	if doc.Find(`a[href="#statement-of-applicability"]`).Length() != 1 {
		t.Fatal("expected quick link to SOA section")
	}
	if doc.Find(`#implementation-mapping`).Length() != 1 {
		t.Fatal("expected implementation mapping section anchor")
	}
	if doc.Find(`#statement-of-applicability`).Length() != 1 {
		t.Fatal("expected SOA section anchor")
	}
}

// ── requirementLinksSection (link-search input) ────────────────────────────────

// TestRequirementLinksSection_MeasureSearchHasHxGet guards two regressions in the
// measure link-search input:
//   - hx-get must be present and non-empty. Passing the URL as templ.SafeURL inside
//     an Attrs map silently drops it (templ.RenderAttributes type-switches on the
//     exact dynamic type and has no SafeURL case), so htmx never fired a request.
//   - the live-search "text-[13px]" class must survive. It used to be passed via
//     Attrs["class"], producing a duplicate class attribute the HTML parser discards;
//     it now goes through InputProps.Class so it merges into the single class attr.
func TestRequirementLinksSection_MeasureSearchHasHxGet(t *testing.T) {
	id := uuid.New()
	doc := renderToDoc(t, requirementLinksSection(RequirementEditVM{Requirement: db.Requirement{ID: id}}))

	input := doc.Find("#measure-link-search")
	if input.Length() != 1 {
		t.Fatalf("expected measure-link-search input, found %d", input.Length())
	}
	hxGet, ok := input.Attr("hx-get")
	if !ok || hxGet == "" {
		t.Fatalf("hx-get missing or empty (SafeURL-in-Attrs regression): %q", hxGet)
	}
	if !strings.Contains(hxGet, "/compliance/requirements/"+id.String()+"/measures/search") {
		t.Errorf("unexpected hx-get target: %q", hxGet)
	}
	if class, _ := input.Attr("class"); !strings.Contains(class, "text-[13px]") {
		t.Errorf("text-[13px] class dropped (duplicate-class regression): %q", class)
	}
}
