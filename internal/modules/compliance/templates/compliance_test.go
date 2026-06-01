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
