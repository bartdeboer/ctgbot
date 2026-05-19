package languagedetect

import (
	"strings"

	"github.com/abadojack/whatlanggo"
)

var languageCodes = map[string]whatlanggo.Lang{
	"de": whatlanggo.Deu,
	"en": whatlanggo.Eng,
	"es": whatlanggo.Spa,
	"fr": whatlanggo.Fra,
	"nl": whatlanggo.Nld,
	"ru": whatlanggo.Rus,
	"uk": whatlanggo.Ukr,
}

func Detect(text string, candidates []string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	whitelist := map[whatlanggo.Lang]bool{}
	for _, candidate := range candidates {
		lang, ok := languageCodes[normalizeCode(candidate)]
		if ok {
			whitelist[lang] = true
		}
	}
	if len(whitelist) == 0 {
		return "", false
	}
	info := whatlanggo.DetectWithOptions(text, whatlanggo.Options{Whitelist: whitelist})
	if !info.IsReliable() {
		return "", false
	}
	code := strings.TrimSpace(info.Lang.Iso6391())
	if code == "" {
		return "", false
	}
	return code, true
}

func normalizeCode(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if idx := strings.IndexAny(value, "-_"); idx >= 0 {
		value = value[:idx]
	}
	return value
}
