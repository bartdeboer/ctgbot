package markdown

import "strings"

func renderTextSegments(segments []segment) string {
	var b strings.Builder
	for _, seg := range segments {
		b.WriteString(seg.Text)
	}
	return strings.TrimSpace(b.String())
}

func renderedTextSegmentLen(seg segment) int {
	return textLen(seg.Text)
}

func renderTextCodeLines(lines []string) string {
	return strings.Join(lines, "\n")
}
