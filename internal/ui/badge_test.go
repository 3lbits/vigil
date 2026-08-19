package ui

import (
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"
	"github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/3lbits/vigil/internal/locale"
)

func TestBadgeTagMatchesBadge(t *testing.T) {
	cases := []struct {
		oldVariant string
		newColor   TagColor
		text       string
	}{
		{"success", TagSuccess, "x"},
		{"implemented", TagSuccess, "x"},
		{"warning", TagWarning, "x"},
		{"in_progress", TagWarning, "x"},
		{"danger", TagDanger, "x"},
		{"deprecated", TagDanger, "x"},
		{"default", TagNeutral, "x"},
	}

	for _, tc := range cases {
		t.Run(tc.oldVariant, func(t *testing.T) {
			got := render(t, BadgeTag(tc.newColor, tc.text))
			want := render(t, Badge(BadgeProps{Variant: tc.oldVariant}), templ.Raw(tc.text))
			if got != want {
				t.Errorf("mapping %q -> %q with text %q:\n got: %s\nwant: %s",
					tc.oldVariant, tc.newColor, tc.text, got, want)
			}
		})
	}
}

func TestNeutralCollapsesStoneToSand(t *testing.T) {
	old := render(t, Badge(BadgeProps{Variant: "neutral"}), templ.Raw("x"))
	if !strings.Contains(old, "bg-stone-100") {
		t.Fatal("precondition failed: old neutral no longer uses stone")
	}
	new := render(t, BadgeTag(TagNeutral, "x"))
	if !strings.Contains(new, "bg-sand-100") {
		t.Error("TagNeutral should use sand")
	}
}

func TestStatusBadge(t *testing.T) {
	ctx := enCtx(t)

	cases := []struct {
		status string
		text   string
		color  TagColor
	}{
		{"implemented", "Implemented", TagSuccess},
		{"in_progress", "In progress", TagWarning},
		{"planned", "Planned", TagNeutral},
		{"deprecated", "Deprecated", TagDanger},
		{"not_a_status", "not_a_status", TagNeutral},
	}

	for _, tc := range cases {
		t.Run(tc.status, func(t *testing.T) {
			got := renderCtx(t, ctx, StatusBadge(tc.status))
			want := renderCtx(t, ctx, BadgeTag(tc.color, tc.text))
			if got != want {
				t.Errorf("status %q:\n got: %s\nwant: %s", tc.status, got, want)
			}
		})
	}
}

func renderCtx(t *testing.T, ctx context.Context, c templ.Component, child ...templ.Component) string {
	t.Helper()
	if len(child) > 0 {
		ctx = templ.WithChildren(ctx, child[0])
	}
	var buf strings.Builder
	if err := c.Render(ctx, &buf); err != nil {
		t.Fatalf("render: %v", err)
	}
	return buf.String()
}

func render(t *testing.T, c templ.Component, child ...templ.Component) string {
	t.Helper()
	return renderCtx(t, context.Background(), c, child...)
}

func enCtx(t *testing.T) context.Context {
	t.Helper()
	bundle, err := locale.NewBundle()
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	loc := i18n.NewLocalizer(bundle, locale.LangEN, locale.DefaultLang)
	return locale.SetLocalizer(context.Background(), loc, locale.LangEN)
}
