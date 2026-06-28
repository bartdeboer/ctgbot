package telegram

import (
	"errors"
	"io"
	"strings"

	"golang.org/x/net/html"
)

// Telegram's HTML parse mode is a small formatting language, not a general
// HTML renderer. Keep the accepted set explicit so text/html can fall back
// before Telegram rejects unsupported document tags such as <p> or <table>.
func isTelegramHTMLSubset(text string) bool {
	z := html.NewTokenizer(strings.NewReader(text))
	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return errors.Is(z.Err(), io.EOF)
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			if !isTelegramHTMLTag(tok) {
				return false
			}
		}
	}
}

func isTelegramHTMLTag(tok html.Token) bool {
	switch strings.ToLower(tok.Data) {
	case "b", "strong", "i", "em", "u", "ins", "s", "strike", "del", "tg-spoiler", "a", "tg-emoji", "code", "pre", "blockquote":
		return true
	case "span":
		for _, attr := range tok.Attr {
			if strings.EqualFold(attr.Key, "class") && strings.TrimSpace(attr.Val) == "tg-spoiler" {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func plainTextFromHTML(text string) string {
	z := html.NewTokenizer(strings.NewReader(text))
	var b strings.Builder
	skipDepth := 0
	lastWasSpace := true
	lastWasNewline := true

	writeSpace := func() {
		if !lastWasSpace && !lastWasNewline {
			b.WriteByte(' ')
			lastWasSpace = true
		}
	}
	writeNewline := func() {
		out := strings.TrimRight(b.String(), " \t")
		b.Reset()
		b.WriteString(out)
		if !lastWasNewline && b.Len() > 0 {
			b.WriteByte('\n')
			lastWasNewline = true
			lastWasSpace = true
		}
	}
	writeTab := func() {
		if !lastWasNewline && b.Len() > 0 {
			b.WriteByte('\t')
			lastWasSpace = true
		}
	}
	writeText := func(s string) {
		for _, part := range strings.Fields(s) {
			if !lastWasSpace && !lastWasNewline {
				b.WriteByte(' ')
			}
			b.WriteString(part)
			lastWasSpace = false
			lastWasNewline = false
		}
	}

	for {
		tt := z.Next()
		switch tt {
		case html.ErrorToken:
			return strings.TrimSpace(b.String())
		case html.TextToken:
			if skipDepth == 0 {
				writeText(html.UnescapeString(string(z.Text())))
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			tok := z.Token()
			name := strings.ToLower(tok.Data)
			if name == "script" || name == "style" || name == "head" {
				skipDepth++
				continue
			}
			if skipDepth > 0 {
				continue
			}
			switch name {
			case "br":
				writeNewline()
			case "p", "div", "section", "article", "header", "footer", "table", "thead", "tbody", "tfoot", "tr", "ul", "ol", "li", "blockquote", "pre":
				writeNewline()
			case "td", "th":
				writeTab()
			case "span", "b", "strong", "i", "em", "u", "ins", "s", "strike", "del", "a", "code", "tg-spoiler", "tg-emoji":
				// Inline text only.
			default:
				writeSpace()
			}
		case html.EndTagToken:
			tok := z.Token()
			name := strings.ToLower(tok.Data)
			if skipDepth > 0 {
				if name == "script" || name == "style" || name == "head" {
					skipDepth--
				}
				continue
			}
			switch name {
			case "p", "div", "section", "article", "header", "footer", "table", "thead", "tbody", "tfoot", "tr", "ul", "ol", "li", "blockquote", "pre":
				writeNewline()
			case "td", "th":
				writeTab()
			}
		}
	}
}
