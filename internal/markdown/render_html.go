package markdown

import (
	"html"
	"strings"
)

func renderHTMLSegments(segments []segment) string {
	var b strings.Builder
	for _, seg := range segments {
		b.WriteString(renderHTMLSegment(seg))
	}
	return strings.TrimSpace(b.String())
}

func renderHTMLSegment(seg segment) string {
	escaped := html.EscapeString(seg.Text)
	if seg.Style&styleCode != 0 {
		return "<code>" + escaped + "</code>"
	}
	if seg.Style&styleItalic != 0 {
		escaped = "<i>" + escaped + "</i>"
	}
	if seg.Style&styleBold != 0 {
		escaped = "<b>" + escaped + "</b>"
	}
	return escaped
}

func renderedHTMLSegmentLen(seg segment) int {
	return textLen(renderHTMLSegment(seg))
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

func renderHTMLCodeLines(lines []string) string {
	return "<pre><code>" + html.EscapeString(strings.Join(lines, "\n")) + "</code></pre>"
}

func htmlCodeWrapperLen() int {
	return len("<pre><code></code></pre>")
}
