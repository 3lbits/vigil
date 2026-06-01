package locale

import (
	"net/http"
	"strings"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

// LangMiddleware detects the preferred language via cookie → Accept-Language → default,
// then injects a *i18n.Localizer into the request context.
func LangMiddleware(bundle *i18n.Bundle) func(http.Handler) http.Handler {
	supported := []language.Tag{
		language.MustParse(LangNB),
		language.MustParse(LangNN),
		language.MustParse(LangEN),
	}
	matcher := language.NewMatcher(supported)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			lang := detectLang(r, matcher)
			localizer := i18n.NewLocalizer(bundle, lang, DefaultLang)
			ctx := SetLocalizer(r.Context(), localizer, lang)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func detectLang(r *http.Request, matcher language.Matcher) string {
	if c, err := r.Cookie(CookieName); err == nil {
		v := strings.TrimSpace(c.Value)
		if v == LangNB || v == LangNN || v == LangEN {
			return v
		}
	}
	if accept := r.Header.Get("Accept-Language"); accept != "" {
		tags, _, err := language.ParseAcceptLanguage(accept)
		if err == nil && len(tags) > 0 {
			tag, _, _ := matcher.Match(tags...)
			return tag.String()
		}
	}
	return DefaultLang
}
