package locale

import (
	"context"

	"github.com/nicksnyder/go-i18n/v2/i18n"
)

type ctxKey struct{}
type ctxLangKey struct{}

// SetLocalizer stores both the localizer and the resolved language string in ctx.
func SetLocalizer(ctx context.Context, l *i18n.Localizer, lang string) context.Context {
	ctx = context.WithValue(ctx, ctxKey{}, l)
	ctx = context.WithValue(ctx, ctxLangKey{}, lang)
	return ctx
}

// FromContext returns the Localizer stored in ctx, or nil if not set.
func FromContext(ctx context.Context) *i18n.Localizer {
	l, _ := ctx.Value(ctxKey{}).(*i18n.Localizer)
	return l
}

// LangFromContext returns the resolved language tag string, or DefaultLang if not set.
func LangFromContext(ctx context.Context) string {
	s, _ := ctx.Value(ctxLangKey{}).(string)
	if s == "" {
		return DefaultLang
	}
	return s
}
