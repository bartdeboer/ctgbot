package markdown

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

// Chunked splits a document into semantic chunk documents using stable text-mode
// sizing. Rendering-specific overflow is handled later at the callsite.
func (d *Document) Chunked(chunkSize int) []*Document {
	if d == nil {
		return []*Document{{}}
	}
	if chunkSize <= 0 {
		chunkSize = 3500
	}
	if d.Size() <= chunkSize {
		return []*Document{CloneDocument(d)}
	}
	lines := d.GetLines()
	if len(lines) == 0 {
		return []*Document{{}}
	}
	var out []*Document
	for start := 0; start < len(lines); {
		end := fitChunkLineWindow(d, lines, start, chunkSize)
		if end > start {
			out = append(out, d.LineSlice(lines[start].StartPos, lines[end-1].EndPos))
			start = end
			continue
		}
		block, line := findLineOwner(d, lines[start])
		parts := splitOversizedLineDocuments(block, line, chunkSize)
		out = append(out, parts...)
		start++
	}
	if len(out) == 0 {
		return []*Document{{}}
	}
	return out
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

func fitChunkLineWindow(doc *Document, lines []*LineNode, start int, limit int) int {
	best := start
	for end := start + 1; end <= len(lines); end++ {
		sliced := doc.LineSlice(lines[start].StartPos, lines[end-1].EndPos)
		if sliced.Size() > limit {
			break
		}
		best = end
	}
	return best
}

func splitOversizedLineDocuments(block *BlockNode, line *LineNode, limit int) []*Document {
	if block == nil || line == nil {
		return []*Document{{}}
	}
	if block.Kind == CodeBlock {
		return splitOversizedCodeLineDocuments(block, line, limit)
	}
	return splitOversizedParagraphLineDocuments(line, limit)
}

func splitOversizedParagraphLineDocuments(line *LineNode, limit int) []*Document {
	segments := flattenLineSegments(line.Spans, 0)
	parts := splitSegments(segments, limit)
	out := make([]*Document, 0, len(parts))
	for _, part := range parts {
		lineCopy := &LineNode{StartPos: line.StartPos, EndPos: line.EndPos, Spans: segmentsToSpans(part)}
		block := &BlockNode{Kind: ParagraphBlock, Lines: []*LineNode{lineCopy}, Span: Span{Start: lineCopy.StartPos, End: lineCopy.EndPos}}
		out = append(out, &Document{Blocks: []*BlockNode{block}, Span: block.Span})
	}
	return out
}

func splitOversizedCodeLineDocuments(block *BlockNode, line *LineNode, limit int) []*Document {
	text := renderTextLine(line)
	var out []*Document
	for text != "" {
		part, rest := splitTextForLimit(text, limit)
		if part == "" {
			part, rest = forceSplit(text, maxInt(1, limit))
		}
		lineCopy := &LineNode{StartPos: line.StartPos, EndPos: line.EndPos, Spans: []*SpanNode{{Kind: TextSpan, Text: part, Span: Span{Start: line.StartPos, End: line.EndPos}}}}
		blockCopy := &BlockNode{Kind: CodeBlock, Meta: cloneMeta(block.Meta), Lines: []*LineNode{lineCopy}, Span: Span{Start: lineCopy.StartPos, End: lineCopy.EndPos}}
		out = append(out, &Document{Blocks: []*BlockNode{blockCopy}, Span: blockCopy.Span})
		text = rest
	}
	return out
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

func splitSegments(segments []segment, limit int) [][]segment {
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
			available := limit - currentLen - textSegmentWrapperLen(seg.Style)
			if available <= 0 {
				flush()
				available = limit - textSegmentWrapperLen(seg.Style)
			}
			part, rest := splitTextForLimit(remaining, available)
			if part == "" {
				part, rest = forceSplit(remaining, maxInt(1, available))
			}
			piece := segment{Text: part, Style: seg.Style}
			pieceLen := textLen(renderTextSegment(piece))
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

func segmentsToSpans(segments []segment) []*SpanNode {
	out := make([]*SpanNode, 0, len(segments))
	for _, seg := range segments {
		out = append(out, segmentToSpan(seg))
	}
	return out
}

func segmentToSpan(seg segment) *SpanNode {
	leaf := &SpanNode{Kind: TextSpan, Text: seg.Text}
	if seg.Style&styleCode != 0 {
		return &SpanNode{Kind: InlineCodeSpan, Text: seg.Text}
	}
	current := leaf
	if seg.Style&styleItalic != 0 {
		current = &SpanNode{Kind: ItalicSpan, Children: []*SpanNode{current}}
	}
	if seg.Style&styleBold != 0 {
		current = &SpanNode{Kind: BoldSpan, Children: []*SpanNode{current}}
	}
	return current
}

func renderTextSegment(seg segment) string {
	text := seg.Text
	if seg.Style&styleCode != 0 {
		return "`" + text + "`"
	}
	if seg.Style&styleItalic != 0 {
		text = "*" + text + "*"
	}
	if seg.Style&styleBold != 0 {
		text = "**" + text + "**"
	}
	return text
}

func textSegmentWrapperLen(style inlineStyle) int {
	if style&styleCode != 0 {
		return len("``")
	}
	n := 0
	if style&styleItalic != 0 {
		n += len("**")
	}
	if style&styleBold != 0 {
		n += len("****")
	}
	return n
}
