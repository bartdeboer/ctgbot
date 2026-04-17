package markdown

import "strings"

func renderMarkdownV2Segments(segments []segment) string {
	var b strings.Builder
	for _, seg := range segments {
		b.WriteString(renderMarkdownV2Segment(seg))
	}
	return strings.TrimSpace(b.String())
}

func renderMarkdownV2Segment(seg segment) string {
	if seg.Style&styleCode != 0 {
		return "`" + escapeMarkdownV2Code(seg.Text) + "`"
	}
	text := escapeMarkdownV2Text(seg.Text)
	if seg.Style&styleItalic != 0 {
		text = "_" + text + "_"
	}
	if seg.Style&styleBold != 0 {
		text = "*" + text + "*"
	}
	return text
}

func renderedMarkdownV2SegmentLen(seg segment) int {
	return textLen(renderMarkdownV2Segment(seg))
}

func markdownV2SegmentWrapperLen(style inlineStyle) int {
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

func renderMarkdownV2CodeLines(lines []string, info string) string {
	body := escapeMarkdownV2Code(strings.Join(lines, "\n"))
	info = strings.TrimSpace(info)
	if info == "" {
		return "```\n" + body + "\n```"
	}
	return "```" + escapeMarkdownV2Code(info) + "\n" + body + "\n```"
}

func markdownV2CodeWrapperLen(info string) int {
	info = strings.TrimSpace(info)
	if info == "" {
		return len("```\n\n```")
	}
	return len("```\n\n```") + textLen(escapeMarkdownV2Code(info))
}

func escapeMarkdownV2Text(text string) string {
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

func escapeMarkdownV2Code(text string) string {
	var b strings.Builder
	for _, r := range text {
		if r == '\\' || r == '`' {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
