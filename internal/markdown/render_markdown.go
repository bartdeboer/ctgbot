package markdown

import "strings"

func renderMarkdownDocument(doc *Document) string {
	if doc == nil || len(doc.Blocks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(doc.Blocks))
	for _, block := range doc.Blocks {
		if block == nil {
			continue
		}
		parts = append(parts, renderMarkdownBlock(block))
	}
	return strings.TrimRight(strings.Join(parts, "\n"), "\n")
}

func renderMarkdownBlock(block *BlockNode) string {
	switch block.Kind {
	case CodeBlock:
		body := renderMarkdownCodeLines(block.Lines)
		info := ""
		if block.Meta != nil {
			info = strings.TrimSpace(block.Meta["info"])
		}
		return wrapMarkdownCodeBlock(body, info)
	case HeadingBlock:
		return "*" + renderMarkdownLines(block.Lines) + "*\n"
	default:
		return renderMarkdownLines(block.Lines) + "\n"
	}
}

func renderMarkdownLines(lines []*LineNode) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		parts = append(parts, renderMarkdownLine(line))
	}
	return strings.Join(parts, "\n")
}

func renderMarkdownCodeLines(lines []*LineNode) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		parts = append(parts, renderMarkdownCodeLine(line))
	}
	return strings.Join(parts, "\n")
}

func renderMarkdownLine(line *LineNode) string {
	if line == nil {
		return ""
	}
	var b strings.Builder
	for _, span := range line.Spans {
		b.WriteString(renderMarkdownSpan(span))
	}
	return b.String()
}

func renderMarkdownCodeLine(line *LineNode) string {
	if line == nil {
		return ""
	}
	var b strings.Builder
	for _, span := range line.Spans {
		b.WriteString(renderMarkdownCodeSpan(span))
	}
	return b.String()
}

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

func renderMarkdownCodeSpan(span *SpanNode) string {
	if span == nil {
		return ""
	}
	if len(span.Children) > 0 {
		var b strings.Builder
		for _, child := range span.Children {
			b.WriteString(renderMarkdownCodeSpan(child))
		}
		return b.String()
	}
	return escapeMarkdownCode(span.Text)
}

func renderMarkdownChildren(children []*SpanNode) string {
	var b strings.Builder
	for _, child := range children {
		b.WriteString(renderMarkdownSpan(child))
	}
	return b.String()
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
