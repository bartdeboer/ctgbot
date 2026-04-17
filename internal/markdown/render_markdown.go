package markdown

import "strings"

func renderMarkdownSpan(span *SpanNode) string {
	if span == nil {
		return ""
	}
	switch span.Kind {
	case BoldSpan:
		return "*" + renderMarkdownChildren(span.Children) + "*"
	case ItalicSpan:
		return "_" + renderMarkdownChildren(span.Children) + "_"
	case InlineCodeSpan:
		return "`" + escapeMarkdownCode(span.Text) + "`"
	default:
		return escapeMarkdownText(span.Text)
	}
}

func renderMarkdownChildren(children []*SpanNode) string {
	var b strings.Builder
	for _, child := range children {
		b.WriteString(renderMarkdownSpan(child))
	}
	return b.String()
}

func renderMarkdownSegment(seg segment) string {
	if seg.Style&styleCode != 0 {
		return "`" + escapeMarkdownCode(seg.Text) + "`"
	}
	text := escapeMarkdownText(seg.Text)
	if seg.Style&styleItalic != 0 {
		text = "_" + text + "_"
	}
	if seg.Style&styleBold != 0 {
		text = "*" + text + "*"
	}
	return text
}

func markdownSegmentWrapperLen(style inlineStyle) int {
	if style&styleCode != 0 {
		return len("``")
	}
	n := 0
	if style&styleItalic != 0 {
		n += len("__")
	}
	if style&styleBold != 0 {
		n += len("**")
	}
	return n
}

func wrapMarkdownCodeBlock(body, info string) string {
	if info == "" {
		return "```\n" + body + "\n```"
	}
	return "```" + escapeMarkdownCode(info) + "\n" + body + "\n```"
}

func escapeMarkdownText(text string) string {
	const special = "_*[]()~`>#+-=|{}.!\\"
	var b strings.Builder
	for _, r := range text {
		if strings.ContainsRune(special, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

func escapeMarkdownCode(text string) string {
	var b strings.Builder
	for _, r := range text {
		if r == '\\' || r == '`' {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
