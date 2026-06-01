package locale

import (
	"embed"
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/nicksnyder/go-i18n/v2/i18n"
	"golang.org/x/text/language"
)

//go:embed locales/*.toml
var localeFS embed.FS

const (
	LangNB      = "nb"
	LangNN      = "nn"
	LangEN      = "en"
	DefaultLang = LangNB
	CookieName  = "vigil_lang"
)

// NewBundle loads all embedded locale files and returns a bundle safe for concurrent use.
func NewBundle() (*i18n.Bundle, error) {
	bundle := i18n.NewBundle(language.MustParse(DefaultLang))
	bundle.RegisterUnmarshalFunc("toml", toml.Unmarshal)

	entries, err := localeFS.ReadDir("locales")
	if err != nil {
		return nil, fmt.Errorf("read locale dir: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := localeFS.ReadFile("locales/" + e.Name())
		if err != nil {
			return nil, fmt.Errorf("read locale file %s: %w", e.Name(), err)
		}
		if _, err = bundle.ParseMessageFileBytes(data, e.Name()); err != nil {
			return nil, fmt.Errorf("parse locale file %s: %w", e.Name(), err)
		}
	}
	return bundle, nil
}
