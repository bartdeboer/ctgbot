package markdown

import (
	"html"
	"strings"
)

func renderHTMLDocument(doc *Document) string {
	if doc == nil || len(doc.Blocks) == 0 {
		return ""
	}
	parts := make([]string, 0, len(doc.Blocks))
	for _, block := range doc.Blocks {
		if block == nil {
			continue
		}
		parts = append(parts, renderHTMLBlock(block))
	}
	return strings.TrimRight(strings.Join(parts, "\n"), "\n")
}

func renderHTMLBlock(block *BlockNode) string {
	switch block.Kind {
	case CodeBlock:
		return wrapHTMLCodeBlock(renderHTMLLines(block.Lines))
	default:
		return renderHTMLLines(block.Lines) + "\n"
	}
}

func renderHTMLLines(lines []*LineNode) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		parts = append(parts, renderHTMLLine(line))
	}
	return strings.Join(parts, "\n")
}

func renderHTMLLine(line *LineNode) string {
	if line == nil {
		return ""
	}
	var b strings.Builder
	for _, span := range line.Spans {
		b.WriteString(renderHTMLSpan(span))
	}
	text := b.String()
	if line.HeadingLevel > 0 {
		return "<b>" + text + "</b>"
	}
	return text
}

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

func wrapHTMLCodeBlock(body string) string {
	return "<pre><code>" + body + "</code></pre>"
}
