package markdown

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

func (d *Document) RenderChunked(opts RenderOptions) ([]Chunk, error) {
	if d == nil || d.Root == nil {
		return nil, fmt.Errorf("document is nil")
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	format := opts.Format
	if format == "" {
		format = RenderPlain
	}
	if format != RenderPlain && format != RenderHTML && format != RenderMarkdownV2 {
		return nil, fmt.Errorf("unsupported render format %q", format)
	}
	limit := opts.ChunkSize
	if limit <= 0 {
		limit = 3500
	}

	docs, err := d.Chunk(limit)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return []Chunk{{Text: ""}}, nil
	}

	var out []Chunk
	for _, doc := range docs {
		text, ok, err := renderDocumentWithoutSplitting(doc, format, limit)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, Chunk{Text: text})
			continue
		}
		fallback, err := renderDocumentWithSplitFallback(doc, format, limit)
		if err != nil {
			return nil, err
		}
		out = append(out, fallback...)
	}
	if len(out) == 0 {
		return []Chunk{{Text: ""}}, nil
	}
	return out, nil
}

func renderDocumentWithoutSplitting(doc *Document, format RenderFormat, limit int) (string, bool, error) {
	if doc == nil || doc.Root == nil {
		return "", false, nil
	}
	var pieces []string
	for _, child := range doc.Root.Children {
		piece, err := renderBlockWhole(child, format)
		if err != nil {
			return "", false, err
		}
		if strings.TrimSpace(piece) == "" {
			continue
		}
		pieces = append(pieces, piece)
	}
	text := strings.Join(pieces, "\n\n")
	if textLen(text) > limit {
		return "", false, nil
	}
	return text, true, nil
}

func renderDocumentWithSplitFallback(doc *Document, format RenderFormat, limit int) ([]Chunk, error) {
	var pieces []string
	for _, child := range doc.Root.Children {
		blockPieces, err := renderBlock(child, format, limit)
		if err != nil {
			return nil, err
		}
		for _, piece := range blockPieces {
			if strings.TrimSpace(piece) == "" {
				continue
			}
			pieces = append(pieces, piece)
		}
	}
	if len(pieces) == 0 {
		return nil, nil
	}
	var chunks []Chunk
	current := ""
	for _, piece := range pieces {
		if current == "" {
			current = piece
			continue
		}
		candidate := current + "\n\n" + piece
		if textLen(candidate) <= limit {
			current = candidate
			continue
		}
		chunks = append(chunks, Chunk{Text: current})
		current = piece
	}
	if current != "" {
		chunks = append(chunks, Chunk{Text: current})
	}
	return chunks, nil
}

func renderBlockWhole(node *Node, format RenderFormat) (string, error) {
	switch node.Kind {
	case NodeParagraph:
		return renderSegments(flattenSegments(node.Children, 0), format), nil
	case NodeCodeBlock:
		info := ""
		if node.Meta != nil {
			info = node.Meta["info"]
		}
		return renderCodeLines(strings.Split(node.Text, "\n"), format, info), nil
	case NodeList:
		return renderListWhole(node, format)
	default:
		return "", fmt.Errorf("unsupported block kind %q", node.Kind)
	}
}

func renderListWhole(list *Node, format RenderFormat) (string, error) {
	var pieces []string
	for _, item := range list.Children {
		text, err := renderListItemWhole(item, format)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		pieces = append(pieces, text)
	}
	return strings.Join(pieces, "\n\n"), nil
}

func renderListItemWhole(item *Node, format RenderFormat) (string, error) {
	if item == nil || item.Kind != NodeListItem {
		return "", nil
	}
	var pieces []string
	for i, child := range item.Children {
		piece, err := renderBlockWhole(child, format)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(piece) == "" {
			continue
		}
		prefix := "  "
		if i == 0 {
			prefix = "• "
		}
		pieces = append(pieces, prefix+indentAfterFirstLine(piece, strings.Repeat(" ", len(prefix))))
	}
	return strings.Join(pieces, "\n\n"), nil
}

func indentAfterFirstLine(text, indent string) string {
	parts := strings.Split(text, "\n")
	if len(parts) <= 1 {
		return text
	}
	for i := 1; i < len(parts); i++ {
		parts[i] = indent + parts[i]
	}
	return strings.Join(parts, "\n")
}

func renderBlock(node *Node, format RenderFormat, limit int) ([]string, error) {
	switch node.Kind {
	case NodeParagraph:
		return renderParagraph(flattenSegments(node.Children, 0), format, limit, ""), nil
	case NodeCodeBlock:
		return renderCodeBlock(node, format, limit), nil
	case NodeList:
		var pieces []string
		for _, item := range node.Children {
			pieces = append(pieces, renderListItem(item, format, limit)...)
		}
		return pieces, nil
	default:
		return nil, fmt.Errorf("unsupported block kind %q", node.Kind)
	}
}

func renderListItem(item *Node, format RenderFormat, limit int) []string {
	if item == nil || item.Kind != NodeListItem || len(item.Children) == 0 {
		return nil
	}
	var pieces []string
	for i, child := range item.Children {
		switch child.Kind {
		case NodeParagraph:
			prefix := "• "
			if i > 0 {
				prefix = "  "
			}
			pieces = append(pieces, renderParagraph(flattenSegments(child.Children, 0), format, limit, prefix)...)
		case NodeCodeBlock:
			for _, piece := range renderCodeBlock(child, format, maxInt(1, limit-2)) {
				pieces = append(pieces, "  "+piece)
			}
		}
	}
	return pieces
}

func flattenSegments(nodes []*Node, style inlineStyle) []segment {
	var out []segment
	for _, node := range nodes {
		switch node.Kind {
		case NodeText:
			out = append(out, segment{Text: node.Text, Style: style})
		case NodeStrong:
			out = append(out, flattenSegments(node.Children, style|styleBold)...)
		case NodeEmphasis:
			out = append(out, flattenSegments(node.Children, style|styleItalic)...)
		case NodeInlineCode:
			out = append(out, segment{Text: node.Text, Style: styleCode})
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

func renderParagraph(segments []segment, format RenderFormat, limit int, prefix string) []string {
	if len(segments) == 0 {
		if prefix == "" {
			return nil
		}
		return []string{prefix}
	}
	var pieces []string
	current := make([]segment, 0, len(segments))
	currentLen := textLen(prefix)

	flush := func() {
		if len(current) == 0 {
			return
		}
		pieces = append(pieces, prefix+renderSegments(current, format))
		current = nil
		currentLen = textLen(prefix)
	}

	for _, seg := range segments {
		remaining := seg.Text
		for remaining != "" {
			available := limit - currentLen
			if available <= 0 {
				flush()
				available = limit - textLen(prefix)
			}
			pieceText, rest := splitTextForLimit(remaining, available-segmentWrapperLen(seg.Style, format))
			if pieceText == "" {
				flush()
				pieceText, rest = splitTextForLimit(remaining, limit-textLen(prefix)-segmentWrapperLen(seg.Style, format))
			}
			piece := segment{Text: pieceText, Style: seg.Style}
			pieceLen := renderedSegmentLen(piece, format)
			if currentLen > textLen(prefix) && currentLen+pieceLen > limit {
				flush()
				continue
			}
			current = append(current, piece)
			currentLen += pieceLen
			remaining = rest
		}
	}
	flush()
	return pieces
}

func renderCodeBlock(node *Node, format RenderFormat, limit int) []string {
	code := ""
	info := ""
	if node != nil {
		code = node.Text
		if node.Meta != nil {
			info = node.Meta["info"]
		}
	}
	lines := strings.Split(code, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	var pieces []string
	current := make([]string, 0, len(lines))
	flush := func() {
		if len(current) == 0 {
			return
		}
		pieces = append(pieces, renderCodeLines(current, format, info))
		current = nil
	}
	for _, line := range lines {
		if len(current) == 0 {
			current = []string{line}
		} else {
			candidate := append(append([]string(nil), current...), line)
			if textLen(renderCodeLines(candidate, format, info)) <= limit {
				current = candidate
				continue
			}
			flush()
			current = []string{line}
		}
		for len(current) > 0 && textLen(renderCodeLines(current, format, info)) > limit {
			part, rest := splitTextForLimit(current[0], limit-codeWrapperLen(format, info))
			if part == "" {
				part, rest = forceSplit(current[0], maxInt(1, limit-codeWrapperLen(format, info)))
			}
			pieces = append(pieces, renderCodeLines([]string{part}, format, info))
			if rest == "" {
				current = current[1:]
				break
			}
			current[0] = rest
		}
	}
	flush()
	return pieces
}

func renderSegments(segments []segment, format RenderFormat) string {
	switch format {
	case RenderHTML:
		return renderHTMLSegments(segments)
	case RenderMarkdownV2:
		return renderMarkdownV2Segments(segments)
	default:
		return renderTextSegments(segments)
	}
}

func renderedSegmentLen(seg segment, format RenderFormat) int {
	switch format {
	case RenderHTML:
		return renderedHTMLSegmentLen(seg)
	case RenderMarkdownV2:
		return renderedMarkdownV2SegmentLen(seg)
	default:
		return renderedTextSegmentLen(seg)
	}
}

func segmentWrapperLen(style inlineStyle, format RenderFormat) int {
	switch format {
	case RenderHTML:
		return htmlSegmentWrapperLen(style)
	case RenderMarkdownV2:
		return markdownV2SegmentWrapperLen(style)
	default:
		return 0
	}
}

func renderCodeLines(lines []string, format RenderFormat, info string) string {
	switch format {
	case RenderHTML:
		return renderHTMLCodeLines(lines)
	case RenderMarkdownV2:
		return renderMarkdownV2CodeLines(lines, info)
	default:
		return renderTextCodeLines(lines)
	}
}

func codeWrapperLen(format RenderFormat, info string) int {
	switch format {
	case RenderHTML:
		return htmlCodeWrapperLen()
	case RenderMarkdownV2:
		return markdownV2CodeWrapperLen(info)
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
	for _, needle := range []string{"\n", ". ", " ", ", ", "; ", ": "} {
		if idx := strings.LastIndex(window, needle); idx > 0 {
			if needle == "\n" {
				return idx
			}
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
