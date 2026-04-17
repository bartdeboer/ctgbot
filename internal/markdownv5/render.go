package markdownv5

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

type RenderFormat string

const (
	RenderPlain      RenderFormat = "plain"
	RenderHTML       RenderFormat = "html"
	RenderMarkdownV2 RenderFormat = "markdown_v2"
)

type RenderOptions struct {
	Format    RenderFormat
	ChunkSize int
}

type Chunk struct {
	Text string
}

func (d *Document) Render(opts RenderOptions) (string, error) {
	if d == nil {
		return "", fmt.Errorf("document is nil")
	}
	format := opts.Format
	if format == "" {
		format = RenderPlain
	}
	return renderDocument(d, format), nil
}

func (d *Document) RenderChunked(opts RenderOptions) ([]Chunk, error) {
	if d == nil {
		return nil, fmt.Errorf("document is nil")
	}
	format := opts.Format
	if format == "" {
		format = RenderPlain
	}
	limit := opts.ChunkSize
	if limit <= 0 {
		limit = 3500
	}
	docs := d.Chunked(limit)
	var chunks []Chunk
	for _, doc := range docs {
		text := renderDocument(doc, format)
		if textLen(text) <= limit {
			chunks = append(chunks, Chunk{Text: text})
			continue
		}
		lines := doc.GetLines()
		if len(lines) != 1 {
			return nil, fmt.Errorf("chunk overflow for multi-line document")
		}
		block, line := findLineOwner(doc, lines[0])
		parts, err := splitOversizedLine(block, line, format, limit)
		if err != nil {
			return nil, err
		}
		chunks = append(chunks, parts...)
	}
	if len(chunks) == 0 {
		return []Chunk{{Text: ""}}, nil
	}
	return chunks, nil
}

func renderDocumentIfFits(doc *Document, format RenderFormat, limit int) (string, bool) {
	text := renderDocument(doc, format)
	if textLen(text) > limit {
		return "", false
	}
	return text, true
}

func renderDocument(doc *Document, format RenderFormat) string {
	if doc == nil || len(doc.Blocks) == 0 {
		return ""
	}
	pieces := make([]string, 0, len(doc.Blocks))
	for _, block := range doc.Blocks {
		if block == nil {
			continue
		}
		pieces = append(pieces, renderBlock(block, format))
	}
	return strings.Join(pieces, "\n\n")
}

func renderBlock(block *BlockNode, format RenderFormat) string {
	switch block.Kind {
	case CodeBlock:
		return renderCodeBlock(block, format)
	default:
		return renderParagraphBlock(block, format)
	}
}

func renderParagraphBlock(block *BlockNode, format RenderFormat) string {
	lines := make([]string, 0, len(block.Lines))
	for _, line := range block.Lines {
		lines = append(lines, renderLine(line, format))
	}
	return strings.Join(lines, "\n")
}

func renderCodeBlock(block *BlockNode, format RenderFormat) string {
	lines := make([]string, 0, len(block.Lines))
	for _, line := range block.Lines {
		lines = append(lines, renderCodeLine(line, format))
	}
	body := strings.Join(lines, "\n")
	info := ""
	if block.Meta != nil {
		info = strings.TrimSpace(block.Meta["info"])
	}
	switch format {
	case RenderHTML:
		return wrapHTMLCodeBlock(body)
	case RenderMarkdownV2:
		return wrapMarkdownCodeBlock(body, info)
	default:
		return body
	}
}

func renderLine(line *LineNode, format RenderFormat) string {
	if line == nil {
		return ""
	}
	var b strings.Builder
	for _, span := range line.Spans {
		b.WriteString(renderSpan(span, format))
	}
	return b.String()
}

func renderCodeLine(line *LineNode, format RenderFormat) string {
	if line == nil {
		return ""
	}
	text := linePlainText(line)
	switch format {
	case RenderHTML:
		return escapeHTMLText(text)
	case RenderMarkdownV2:
		return escapeMarkdownCode(text)
	default:
		return text
	}
}

func renderSpan(span *SpanNode, format RenderFormat) string {
	if span == nil {
		return ""
	}
	switch format {
	case RenderHTML:
		return renderHTMLSpan(span)
	case RenderMarkdownV2:
		return renderMarkdownSpan(span)
	default:
		return renderTextSpan(span)
	}
}

func renderTextSpan(span *SpanNode) string {
	if len(span.Children) == 0 {
		return span.Text
	}
	var b strings.Builder
	for _, child := range span.Children {
		b.WriteString(renderTextSpan(child))
	}
	return b.String()
}

func fitLineWindow(doc *Document, lines []*LineNode, start int, format RenderFormat, limit int) int {
	best := start
	for end := start + 1; end <= len(lines); end++ {
		sliced := doc.LineSlice(lines[start].StartPos, lines[end-1].EndPos)
		if _, ok := renderDocumentIfFits(sliced, format, limit); !ok {
			break
		}
		best = end
	}
	return best
}

func findLineOwner(doc *Document, target *LineNode) (*BlockNode, *LineNode) {
	for _, block := range doc.Blocks {
		for _, line := range block.Lines {
			if line == target {
				return block, line
			}
		}
	}
	return nil, target
}

func splitOversizedLine(block *BlockNode, line *LineNode, format RenderFormat, limit int) ([]Chunk, error) {
	if block == nil || line == nil {
		return nil, fmt.Errorf("missing line owner")
	}
	if block.Kind == CodeBlock {
		return splitOversizedCodeLine(block, line, format, limit), nil
	}
	return splitOversizedParagraphLine(line, format, limit), nil
}

func splitOversizedParagraphLine(line *LineNode, format RenderFormat, limit int) []Chunk {
	segments := flattenLineSegments(line.Spans, 0)
	parts := splitSegments(segments, format, limit)
	out := make([]Chunk, 0, len(parts))
	for _, part := range parts {
		out = append(out, Chunk{Text: renderSegments(part, format)})
	}
	return out
}

func splitOversizedCodeLine(block *BlockNode, line *LineNode, format RenderFormat, limit int) []Chunk {
	text := linePlainText(line)
	info := ""
	if block.Meta != nil {
		info = strings.TrimSpace(block.Meta["info"])
	}
	maxBody := maxInt(1, limit-codeWrapperLen(format, info))
	var out []Chunk
	for text != "" {
		part, rest := splitTextForLimit(text, maxBody)
		if part == "" {
			part, rest = forceSplit(text, maxBody)
		}
		chunkBlock := &BlockNode{Kind: CodeBlock, Meta: cloneMeta(block.Meta), Lines: []*LineNode{{Spans: []*SpanNode{{Kind: TextSpan, Text: part}}}}}
		out = append(out, Chunk{Text: renderCodeBlock(chunkBlock, format)})
		text = rest
	}
	return out
}

type inlineStyle uint8

const (
	styleBold inlineStyle = 1 << iota
	styleItalic
	styleCode
)

type segment struct {
	Text  string
	Style inlineStyle
}

func flattenLineSegments(spans []*SpanNode, style inlineStyle) []segment {
	var out []segment
	for _, span := range spans {
		switch span.Kind {
		case TextSpan:
			out = append(out, segment{Text: span.Text, Style: style})
		case BoldSpan:
			out = append(out, flattenLineSegments(span.Children, style|styleBold)...)
		case ItalicSpan:
			out = append(out, flattenLineSegments(span.Children, style|styleItalic)...)
		case InlineCodeSpan:
			out = append(out, segment{Text: span.Text, Style: styleCode})
		}
	}
	return mergeSegments(out)
}

func mergeSegments(in []segment) []segment {
	out := make([]segment, 0, len(in))
	for _, seg := range in {
		if seg.Text == "" {
			continue
		}
		if len(out) > 0 && out[len(out)-1].Style == seg.Style {
			out[len(out)-1].Text += seg.Text
			continue
		}
		out = append(out, seg)
	}
	return out
}

func splitSegments(segments []segment, format RenderFormat, limit int) [][]segment {
	var out [][]segment
	var current []segment
	currentLen := 0
	flush := func() {
		if len(current) == 0 {
			return
		}
		out = append(out, current)
		current = nil
		currentLen = 0
	}
	for _, seg := range segments {
		remaining := seg.Text
		for remaining != "" {
			available := limit - currentLen - segmentWrapperLen(seg.Style, format)
			if available <= 0 {
				flush()
				available = limit - segmentWrapperLen(seg.Style, format)
			}
			part, rest := splitTextForLimit(remaining, available)
			if part == "" {
				part, rest = forceSplit(remaining, maxInt(1, available))
			}
			piece := segment{Text: part, Style: seg.Style}
			pieceLen := renderedSegmentLen(piece, format)
			if currentLen > 0 && currentLen+pieceLen > limit {
				flush()
				continue
			}
			current = append(current, piece)
			currentLen += pieceLen
			remaining = rest
		}
	}
	flush()
	return out
}

func linePlainText(line *LineNode) string {
	var b strings.Builder
	for _, span := range line.Spans {
		b.WriteString(renderTextSpan(span))
	}
	return b.String()
}

func renderSegments(segments []segment, format RenderFormat) string {
	var b strings.Builder
	for _, seg := range segments {
		switch format {
		case RenderHTML:
			b.WriteString(renderHTMLSegment(seg))
		case RenderMarkdownV2:
			b.WriteString(renderMarkdownSegment(seg))
		default:
			b.WriteString(seg.Text)
		}
	}
	return b.String()
}

func renderedSegmentLen(seg segment, format RenderFormat) int {
	return textLen(renderSegments([]segment{seg}, format))
}

func segmentWrapperLen(style inlineStyle, format RenderFormat) int {
	switch format {
	case RenderHTML:
		return htmlSegmentWrapperLen(style)
	case RenderMarkdownV2:
		return markdownSegmentWrapperLen(style)
	default:
		return 0
	}
}

func codeWrapperLen(format RenderFormat, info string) int {
	switch format {
	case RenderHTML:
		return len("<pre><code></code></pre>")
	case RenderMarkdownV2:
		if info == "" {
			return len("```\n\n```")
		}
		return len("```\n\n```") + textLen(escapeMarkdownCode(info))
	default:
		return 0
	}
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

func textLen(text string) int {
	return utf8.RuneCountInString(text)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
