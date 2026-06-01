package locale_test

import (
	"context"
	"testing"

	"github.com/nicksnyder/go-i18n/v2/i18n"

	"github.com/3lbits/vigil/internal/locale"
)

func ctxForLang(t *testing.T, bundle *i18n.Bundle, lang string) context.Context {
	t.Helper()
	loc := i18n.NewLocalizer(bundle, lang, locale.DefaultLang)
	return locale.SetLocalizer(context.Background(), loc, lang)
}

func TestT(t *testing.T) {
	bundle, err := locale.NewBundle()
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}

	for _, tc := range []struct {
		lang string
		key  string
		want string
	}{
		{locale.LangNB, "nav_measures", "Tiltak"},
		{locale.LangNB, "nav_dashboard", "Oversikt"},
		{locale.LangNB, "action_sign_out", "Logg ut"},
		{locale.LangNB, "login_title", "Logg inn på Vigil"},
		{locale.LangNN, "nav_measures", "Tiltak"},
		{locale.LangNN, "nav_dashboard", "Oversyn"},
		{locale.LangNN, "action_sign_out", "Logg ut"},
		{locale.LangNN, "login_title", "Logg inn i Vigil"},
		{locale.LangEN, "nav_measures", "Measures"},
		{locale.LangEN, "nav_dashboard", "Dashboard"},
		{locale.LangEN, "action_sign_out", "Sign out"},
		{locale.LangEN, "login_title", "Sign in to Vigil"},
	} {
		t.Run(tc.lang+"/"+tc.key, func(t *testing.T) {
			ctx := ctxForLang(t, bundle, tc.lang)
			if got := locale.T(ctx, tc.key); got != tc.want {
				t.Errorf("T(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

func TestTN(t *testing.T) {
	bundle, err := locale.NewBundle()
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}

	for _, tc := range []struct {
		lang  string
		count int
		want  string
	}{
		{locale.LangNB, 1, "1 tiltak"},
		{locale.LangNB, 3, "3 tiltak"},
		{locale.LangNN, 1, "1 tiltak"},
		{locale.LangNN, 3, "3 tiltak"},
		{locale.LangEN, 1, "1 measure"},
		{locale.LangEN, 3, "3 measures"},
	} {
		t.Run(tc.lang, func(t *testing.T) {
			ctx := ctxForLang(t, bundle, tc.lang)
			if got := locale.TN(ctx, "measures_count", tc.count); got != tc.want {
				t.Errorf("TN(%d) = %q, want %q", tc.count, got, tc.want)
			}
		})
	}
}

func TestFallbacks(t *testing.T) {
	bundle, err := locale.NewBundle()
	if err != nil {
		t.Fatalf("NewBundle: %v", err)
	}
	ctx := ctxForLang(t, bundle, locale.LangNB)

	// Missing key returns the ID.
	if got := locale.T(ctx, "bogus_key_xyz"); got != "bogus_key_xyz" {
		t.Errorf("missing key: got %q, want key ID as fallback", got)
	}

	// Empty context (no middleware) returns key ID, never panics.
	if got := locale.T(context.Background(), "nav_measures"); got != "nav_measures" {
		t.Errorf("empty ctx: got %q, want key ID", got)
	}
}

func TestLangFromContext(t *testing.T) {
	if got := locale.LangFromContext(context.Background()); got != locale.DefaultLang {
		t.Errorf("empty ctx: got %q, want %q", got, locale.DefaultLang)
	}
}
