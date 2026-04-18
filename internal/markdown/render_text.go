package markdown

import (
	"strings"
	"unicode/utf8"
)

func renderTextDocument(doc *Document) string {
	if doc == nil || len(doc.Blocks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(doc.Blocks))
	for _, block := range doc.Blocks {
		if block == nil {
			continue
		}
		parts = append(parts, renderTextBlock(block))
	}
	return strings.TrimRight(strings.Join(parts, "\n"), "\n")
}

func renderTextBlock(block *BlockNode) string {
	switch block.Kind {
	case CodeBlock:
		body := renderTextLines(block.Lines)
		info := ""
		if block.Meta != nil {
			info = strings.TrimSpace(block.Meta["info"])
		}
		if info == "" {
			return "```\n" + body + "\n```\n"
		}
		return "```" + info + "\n" + body + "\n```\n"
	case HeadingBlock:
		return strings.Repeat("#", block.HeadingLevel) + " " + renderTextLines(block.Lines) + "\n"
	default:
		return renderTextLines(block.Lines) + "\n"
	}
}

func renderTextLines(lines []*LineNode) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		parts = append(parts, renderTextLine(line))
	}
	return strings.Join(parts, "\n")
}

func renderTextLine(line *LineNode) string {
	if line == nil {
		return ""
	}
	var b strings.Builder
	for _, span := range line.Spans {
		b.WriteString(renderTextSpan(span))
	}
	return b.String()
}

func renderTextSpan(span *SpanNode) string {
	if span == nil {
		return ""
	}
	switch span.Kind {
	case BoldSpan:
		return "**" + renderTextChildren(span.Children) + "**"
	case ItalicSpan:
		return "*" + renderTextChildren(span.Children) + "*"
	case InlineCodeSpan:
		return "`" + span.Text + "`"
	default:
		return span.Text
	}
}

func renderTextChildren(children []*SpanNode) string {
	var b strings.Builder
	for _, child := range children {
		b.WriteString(renderTextSpan(child))
	}
	return b.String()
}

func textLen(text string) int {
	return utf8.RuneCountInString(text)
}

func splitTextForLimit(text string, limit int) (string, string) {
	if text == "" {
		return "", ""
	}
	if limit <= 0 {
		return "", text
	}
	if textLen(text) <= limit {
		return text, ""
	}
	cut := bestCut(text, limit)
	if cut <= 0 {
		return forceSplit(text, limit)
	}
	return strings.TrimSpace(text[:cut]), strings.TrimSpace(text[cut:])
}

func bestCut(text string, limit int) int {
	if textLen(text) <= limit {
		return len(text)
	}
	cut := byteIndexAtRuneLimit(text, limit)
	window := text[:cut]
	for _, needle := range []string{" ", ", ", "; ", ": "} {
		if idx := strings.LastIndex(window, needle); idx > 0 {
			return idx + len(needle) - 1
		}
	}
	return cut
}

func forceSplit(text string, limit int) (string, string) {
	cut := byteIndexAtRuneLimit(text, limit)
	if cut <= 0 || cut >= len(text) {
		return text, ""
	}
	return strings.TrimSpace(text[:cut]), strings.TrimSpace(text[cut:])
}

func byteIndexAtRuneLimit(text string, limit int) int {
	if limit <= 0 {
		return 0
	}
	count := 0
	for idx := range text {
		if count == limit {
			return idx
		}
		count++
	}
	return len(text)
}
