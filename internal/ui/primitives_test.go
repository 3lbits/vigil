package ui

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/nicksnyder/go-i18n/v2/i18n"

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

func renderHTML(t *testing.T, comp interface {
	Render(context.Context, io.Writer) error
}, children ...templ.Component) string {
	t.Helper()
	var b strings.Builder
	ctx := testCtx(t)
	if len(children) > 0 {
		ctx = templ.WithChildren(ctx, templ.Join(children...))
	}
	if err := comp.Render(ctx, &b); err != nil {
		t.Fatalf("render: %v", err)
	}
	return b.String()
}

func assertRenderedWithClass(t *testing.T, html, classToken string) {
	t.Helper()
	if strings.TrimSpace(html) == "" {
		t.Fatal("expected non-empty rendered html")
	}
	if !strings.Contains(html, classToken) {
		t.Fatalf("expected class token %q in rendered html", classToken)
	}
}

func TestButton_RenderSmoke(t *testing.T) {
	html := renderHTML(t, Button(ButtonProps{Variant: "primary", Size: "md"}), templ.Raw("Click"))
	assertRenderedWithClass(t, html, "focus:ring-2")
}

func TestCard_RenderSmoke(t *testing.T) {
	html := renderHTML(t, Card(CardProps{
		Accent: "border-t-red-500",
	}), templ.Raw("Content"))
	assertRenderedWithClass(t, html, "dark:bg-zinc-900")
	assertRenderedWithClass(t, html, "border-t-4")
	assertRenderedWithClass(t, html, "border-t-red-500")
}

func TestField_RenderSmoke(t *testing.T) {
	html := renderHTML(t, Field("field-id", "label_type", true), Input(InputProps{ID: "field-id", Name: "field"}))
	assertRenderedWithClass(t, html, "dark:text-zinc-200")
}

func TestInput_RenderSmoke(t *testing.T) {
	html := renderHTML(t, Input(InputProps{ID: "name", Name: "name"}))
	assertRenderedWithClass(t, html, "focus:ring-2")
}

func TestTextarea_RenderSmoke(t *testing.T) {
	html := renderHTML(t, Textarea(TextareaProps{ID: "desc", Name: "description"}))
	assertRenderedWithClass(t, html, "min-h-[96px]")
}

func TestSelect_RenderSmoke(t *testing.T) {
	html := renderHTML(t, Select(SelectProps{ID: "type", Name: "type"}), templ.Raw(`<option value="">One</option>`))
	assertRenderedWithClass(t, html, "dark:bg-zinc-900")
}

func TestBadge_RenderSmoke(t *testing.T) {
	html := renderHTML(t, Badge(BadgeProps{Variant: "success"}), templ.Raw("OK"))
	assertRenderedWithClass(t, html, "rounded-full")
}

func TestTable_RenderSmoke(t *testing.T) {
	html := renderHTML(t, Table(TableProps{}), templ.Raw("<tbody><tr><td>Value</td></tr></tbody>"))
	assertRenderedWithClass(t, html, "border-collapse")
}

func TestTHead_RenderSmoke(t *testing.T) {
	html := renderHTML(t, THead(THeadProps{}), templ.Raw("<tr><th>Header</th></tr>"))
	assertRenderedWithClass(t, html, "dark:bg-zinc-900")
}

func TestTr_RenderSmoke(t *testing.T) {
	html := renderHTML(t, Tr(TrProps{}), templ.Raw("<td>Value</td>"))
	assertRenderedWithClass(t, html, "dark:border-zinc-700")
}

func TestTh_RenderSmoke(t *testing.T) {
	html := renderHTML(t, Th(ThProps{}), templ.Raw("Header"))
	assertRenderedWithClass(t, html, "text-left")
}

func TestTd_RenderSmoke(t *testing.T) {
	html := renderHTML(t, Td(TdProps{}), templ.Raw("Value"))
	assertRenderedWithClass(t, html, "align-top")
}

func TestEmptyState_RenderSmoke(t *testing.T) {
	html := renderHTML(t, EmptyState(EmptyStateProps{TestID: "empty"}), templ.Raw("Nothing here"))
	assertRenderedWithClass(t, html, "border-dashed")
}
