package markdownv5

func (d *Document) Chunked(chunkSize int) []*Document {
	if d == nil {
		return []*Document{{}}
	}
	if chunkSize <= 0 {
		chunkSize = 3500
	}
	if text, ok := renderDocumentIfFits(d, RenderPlain, chunkSize); ok {
		_ = text
		return []*Document{CloneDocument(d)}
	}
	lines := d.GetLines()
	if len(lines) == 0 {
		return []*Document{{}}
	}
	var out []*Document
	for start := 0; start < len(lines); {
		end := fitLineWindow(d, lines, start, RenderPlain, chunkSize)
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
	parts := splitSegments(segments, RenderPlain, limit)
	out := make([]*Document, 0, len(parts))
	for _, part := range parts {
		lineCopy := &LineNode{StartPos: line.StartPos, EndPos: line.EndPos, Spans: segmentsToSpans(part)}
		block := &BlockNode{Kind: ParagraphBlock, Lines: []*LineNode{lineCopy}, Span: Span{Start: lineCopy.StartPos, End: lineCopy.EndPos}}
		out = append(out, &Document{Blocks: []*BlockNode{block}, Span: block.Span})
	}
	return out
}

func splitOversizedCodeLineDocuments(block *BlockNode, line *LineNode, limit int) []*Document {
	text := linePlainText(line)
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
