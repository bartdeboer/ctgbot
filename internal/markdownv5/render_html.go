package markdownv5

import (
	"html"
	"strings"
)

func renderHTMLSpan(span *SpanNode) string {
	if span == nil {
		return ""
	}
	switch span.Kind {
	case BoldSpan:
		return "<b>" + renderHTMLChildren(span.Children) + "</b>"
	case ItalicSpan:
		return "<i>" + renderHTMLChildren(span.Children) + "</i>"
	case InlineCodeSpan:
		return "<code>" + html.EscapeString(span.Text) + "</code>"
	default:
		return html.EscapeString(span.Text)
	}
}

func renderHTMLChildren(children []*SpanNode) string {
	var b strings.Builder
	for _, child := range children {
		b.WriteString(renderHTMLSpan(child))
	}
	return b.String()
}

func renderHTMLSegment(seg segment) string {
	text := html.EscapeString(seg.Text)
	if seg.Style&styleCode != 0 {
		return "<code>" + text + "</code>"
	}
	if seg.Style&styleItalic != 0 {
		text = "<i>" + text + "</i>"
	}
	if seg.Style&styleBold != 0 {
		text = "<b>" + text + "</b>"
	}
	return text
}

func htmlSegmentWrapperLen(style inlineStyle) int {
	if style&styleCode != 0 {
		return len("<code></code>")
	}
	n := 0
	if style&styleItalic != 0 {
		n += len("<i></i>")
	}
	if style&styleBold != 0 {
		n += len("<b></b>")
	}
	return n
}

func wrapHTMLCodeBlock(body string) string {
	return "<pre><code>" + body + "</code></pre>"
}

func escapeHTMLText(text string) string {
	return html.EscapeString(text)
}
