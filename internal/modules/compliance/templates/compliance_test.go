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
	if doc.Find(`meter[low="50"][high="80"][optimum="100"]`).Length() != 2 {
		t.Error("framework coverage should use native meters")
	}
}

func TestFrameworkDetail_CoverageMeter(t *testing.T) {
	vm := FrameworkDetailVM{Requirements: []db.Requirement{{}, {}}, CoveredReqs: 1}
	doc := renderToDoc(t, FrameworkDetail(vm))
	if doc.Find(`meter[value="50"][max="100"][low="50"][high="80"][optimum="100"]`).Length() != 1 {
		t.Error("framework detail should show 50% coverage with shared meter thresholds")
	}
}

func TestFrameworkList_EmptyState(t *testing.T) {
	doc := renderToDoc(t, FrameworkList(nil, "", "", ""))

	if doc.Find(".ui2.ui2-compliance").Length() != 1 {
		t.Error("framework list must have a UI2 page root")
	}
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

func TestCompliancePages_UI2RootsAndControls(t *testing.T) {
	id := uuid.New()
	pages := map[string]interface {
		Render(context.Context, io.Writer) error
	}{
		"framework form":   FrameworkForm(nil, "", "", nil),
		"requirement form": RequirementForm(id.String(), nil, "", "", nil),
		"import form":      ImportForm(id.String(), "", "", nil),
		"framework detail": FrameworkDetail(FrameworkDetailVM{Framework: db.Framework{ID: id}}),
		"requirement detail": RequirementDetail(RequirementDetailVM{
			Requirement: db.Requirement{ID: id, FrameworkID: id},
		}),
		"requirement edit": RequirementEditPage(RequirementEditVM{
			Requirement: db.Requirement{ID: id, FrameworkID: id},
		}, "", ""),
	}
	for name, page := range pages {
		t.Run(name, func(t *testing.T) {
			doc := renderToDoc(t, page)
			if got := doc.Find(".ui2.ui2-compliance").Length(); got != 1 {
				t.Errorf("expected one UI2 page root, got %d", got)
			}
		})
	}

	doc := renderToDoc(t, FrameworkList([]FrameworkVM{{
		Framework: db.Framework{ID: id, Name: "ISO"},
		Requirements: []RequirementVM{{
			Requirement: db.Requirement{ID: uuid.New(), Ref: "A.1", Title: "Policy"},
		}},
	}}, id.String(), "", ""))
	if doc.Find("#fw-"+id.String()+` > details[open] > summary`).Length() != 1 {
		t.Error("requested framework must render expanded")
	}
	if doc.Find("#fw-"+id.String()+` > .fw-actions a[href$="/edit"]`).Length() != 1 {
		t.Error("framework actions must remain outside the disclosure")
	}
	closed := renderToDoc(t, FrameworkList([]FrameworkVM{{Framework: db.Framework{ID: id, Name: "ISO"}}}, "", "", ""))
	if closed.Find("#fw-"+id.String()+` > details:not([open]) > summary`).Length() != 1 {
		t.Error("framework without expand parameter must render collapsed")
	}
	if doc.Find(`.ui2-table[aria-label="Requirements"] table caption`).Length() != 1 {
		t.Error("requirements table must retain one accessible label")
	}
	if doc.Find(`.ui2-table[aria-label="Requirements"] th[scope="col"]`).Length() != 4 {
		t.Error("requirements table must retain its column scopes")
	}
	if doc.Find(`form[action$="/delete"] button[data-tone="danger"]`).Length() != 1 {
		t.Error("framework deletion must retain its destructive affordance")
	}
}

func TestComplianceForms_NativeFieldsAndFlash(t *testing.T) {
	id := uuid.New()
	doc := renderToDoc(t, FrameworkForm(nil, "Saved", "success", nil))
	if doc.Find(`form[action="/compliance/frameworks"] input[name="_csrf"]`).Length() != 1 ||
		doc.Find(`input[name="name"][required]`).Length() != 1 ||
		doc.Find(`select[name="framework_type"] option[value="regulation"][selected]`).Length() != 1 {
		t.Error("framework form must retain CSRF, required name, and default framework type")
	}
	if doc.Find(`.ui2-notice[role="status"][data-tone="success"]`).Text() != "Saved" {
		t.Error("success flash must retain its status announcement")
	}
	doc = renderToDoc(t, RequirementForm(id.String(), nil, "Invalid", "error", nil))
	if doc.Find(`form[action="/compliance/frameworks/`+id.String()+`/requirements"] input[name="_csrf"]`).Length() != 1 ||
		doc.Find(`input[name="ref"][required]`).Length() != 1 ||
		doc.Find(`input[name="title"][required]`).Length() != 1 {
		t.Error("requirement form must retain action, CSRF, and required fields")
	}
	if doc.Find(`.ui2-notice[role="alert"][data-tone="danger"]`).Text() != "Invalid" {
		t.Error("error flash must retain its alert announcement")
	}
}

func TestRequirementActions_DestructiveButtons(t *testing.T) {
	id := uuid.New()
	doc := renderToDoc(t, RequirementDetail(RequirementDetailVM{
		Requirement: db.Requirement{ID: id, FrameworkID: uuid.New()}, CanEdit: true,
	}))
	if doc.Find(`form[action$="/delete"] button[data-tone="danger"]`).Length() != 1 {
		t.Error("requirement deletion must retain its destructive affordance")
	}
	doc = renderToDoc(t, RequirementEditPage(RequirementEditVM{
		Requirement:    db.Requirement{ID: id, FrameworkID: uuid.New()},
		LinkedMeasures: []db.ListMeasuresForRequirementRow{{ID: uuid.New(), Name: "Review"}},
	}, "", ""))
	if doc.Find(`form[action$="/delete"] button[data-tone="danger"]`).Length() != 1 {
		t.Error("unlinking a measure must retain its destructive affordance")
	}
}

func TestMeasureSearchFragment_NoPageWrapper(t *testing.T) {
	doc := renderToDoc(t, RequirementMeasureSearchResults(uuid.NewString(), nil, "search"))
	if doc.Find(".ui2").Length() != 0 {
		t.Error("htmx search fragment must not include a page wrapper")
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
//   - the live-search ops-input class must survive in a single class attribute.
//     Passing a second class via Attrs["class"] previously lost the intended class.
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
