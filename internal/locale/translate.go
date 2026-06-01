package locale

import (
	"context"
	"maps"

	"github.com/nicksnyder/go-i18n/v2/i18n"
)

// T translates messageID using the localizer in ctx.
// Returns messageID as fallback if the localizer is absent or the key is missing.
func T(ctx context.Context, messageID string, tplData ...map[string]any) string {
	l := FromContext(ctx)
	if l == nil {
		return messageID
	}
	cfg := &i18n.LocalizeConfig{MessageID: messageID}
	if len(tplData) > 0 {
		cfg.TemplateData = tplData[0]
	}
	s, err := l.Localize(cfg)
	if err != nil {
		return messageID
	}
	return s
}

// TN translates messageID with plural form determined by count.
// Always injects {"Count": count} into template data so messages can use {{.Count}}.
func TN(ctx context.Context, messageID string, count int, tplData ...map[string]any) string {
	l := FromContext(ctx)
	if l == nil {
		return messageID
	}
	data := map[string]any{"Count": count}
	if len(tplData) > 0 {
		maps.Copy(data, tplData[0])
	}
	s, err := l.Localize(&i18n.LocalizeConfig{
		MessageID:    messageID,
		PluralCount:  count,
		TemplateData: data,
	})
	if err != nil {
		return messageID
	}
	return s
}
