package markdown

type Position struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

type Span struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type BlockKind string

const (
	ParagraphBlock BlockKind = "paragraph"
	HeadingBlock   BlockKind = "heading"
	CodeBlock      BlockKind = "code_block"
)

type SpanKind string

const (
	TextSpan       SpanKind = "text"
	BoldSpan       SpanKind = "bold"
	ItalicSpan     SpanKind = "italic"
	InlineCodeSpan SpanKind = "inline_code"
)

type Document struct {
	Blocks []*BlockNode `json:"blocks,omitempty"`
	Span   Span         `json:"span"`
}

type BlockNode struct {
	Kind         BlockKind         `json:"kind"`
	HeadingLevel int               `json:"heading_level,omitempty"`
	Lines        []*LineNode       `json:"lines,omitempty"`
	Meta         map[string]string `json:"meta,omitempty"`
	Span         Span              `json:"span"`
}

type LineNode struct {
	StartPos Position    `json:"start_pos"`
	EndPos   Position    `json:"end_pos"`
	Spans    []*SpanNode `json:"spans,omitempty"`
}

type SpanNode struct {
	Kind     SpanKind    `json:"kind"`
	Text     string      `json:"text,omitempty"`
	Children []*SpanNode `json:"children,omitempty"`
	Span     Span        `json:"span"`
}

func (d *Document) GetLines() []*LineNode {
	if d == nil {
		return nil
	}
	var out []*LineNode
	for _, block := range d.Blocks {
		out = append(out, block.GetLines()...)
	}
	return out
}

func (d *Document) Size() int {
	if d == nil {
		return 0
	}
	total := 0
	for i, block := range d.Blocks {
		if block == nil {
			continue
		}
		if i > 0 {
			total += 2
		}
		total += block.Size()
	}
	return total
}

func (b *BlockNode) GetLines() []*LineNode {
	if b == nil {
		return nil
	}
	out := make([]*LineNode, 0, len(b.Lines))
	out = append(out, b.Lines...)
	return out
}

func (b *BlockNode) Size() int {
	if b == nil {
		return 0
	}
	total := 0
	if b.Kind == HeadingBlock && b.HeadingLevel > 0 {
		total += b.HeadingLevel + 1
	}
	for i, line := range b.Lines {
		if line == nil {
			continue
		}
		if i > 0 {
			total++
		}
		total += line.Size()
	}
	return total
}

func (l *LineNode) Size() int {
	if l == nil {
		return 0
	}
	total := 0
	for _, span := range l.Spans {
		total += span.Size()
	}
	return total
}

func (s *SpanNode) Size() int {
	if s == nil {
		return 0
	}
	if len(s.Children) == 0 {
		return textLen(s.Text)
	}
	total := 0
	for _, child := range s.Children {
		total += child.Size()
	}
	return total
}

func (d *Document) LineSlice(startPos, endPos Position) *Document {
	if d == nil {
		return &Document{}
	}
	if endPos.Offset < startPos.Offset {
		startPos, endPos = endPos, startPos
	}
	var blocks []*BlockNode
	for _, block := range d.Blocks {
		sliced := block.LineSlice(startPos, endPos)
		if sliced != nil {
			blocks = append(blocks, sliced)
		}
	}
	out := &Document{Blocks: blocks}
	if len(blocks) > 0 {
		out.Span = Span{Start: blocks[0].Span.Start, End: blocks[len(blocks)-1].Span.End}
	}
	return out
}

func (b *BlockNode) LineSlice(startPos, endPos Position) *BlockNode {
	if b == nil {
		return nil
	}
	var lines []*LineNode
	for _, line := range b.Lines {
		if line == nil {
			continue
		}
		if line.EndPos.Offset <= startPos.Offset || line.StartPos.Offset >= endPos.Offset {
			continue
		}
		lines = append(lines, CloneLine(line))
	}
	if len(lines) == 0 {
		return nil
	}
	out := &BlockNode{Kind: b.Kind, HeadingLevel: b.HeadingLevel, Lines: lines, Meta: cloneMeta(b.Meta)}
	out.Span = Span{Start: lines[0].StartPos, End: lines[len(lines)-1].EndPos}
	return out
}

func CloneDocument(doc *Document) *Document {
	if doc == nil {
		return nil
	}
	out := &Document{Span: doc.Span}
	if len(doc.Blocks) > 0 {
		out.Blocks = make([]*BlockNode, 0, len(doc.Blocks))
		for _, block := range doc.Blocks {
			out.Blocks = append(out.Blocks, CloneBlock(block))
		}
	}
	return out
}

func CloneBlock(block *BlockNode) *BlockNode {
	if block == nil {
		return nil
	}
	out := &BlockNode{Kind: block.Kind, HeadingLevel: block.HeadingLevel, Span: block.Span, Meta: cloneMeta(block.Meta)}
	if len(block.Lines) > 0 {
		out.Lines = make([]*LineNode, 0, len(block.Lines))
		for _, line := range block.Lines {
			out.Lines = append(out.Lines, CloneLine(line))
		}
	}
	return out
}

func CloneLine(line *LineNode) *LineNode {
	if line == nil {
		return nil
	}
	out := &LineNode{StartPos: line.StartPos, EndPos: line.EndPos}
	if len(line.Spans) > 0 {
		out.Spans = make([]*SpanNode, 0, len(line.Spans))
		for _, span := range line.Spans {
			out.Spans = append(out.Spans, CloneSpan(span))
		}
	}
	return out
}

func CloneSpan(span *SpanNode) *SpanNode {
	if span == nil {
		return nil
	}
	out := &SpanNode{Kind: span.Kind, Text: span.Text, Span: span.Span}
	if len(span.Children) > 0 {
		out.Children = make([]*SpanNode, 0, len(span.Children))
		for _, child := range span.Children {
			out.Children = append(out.Children, CloneSpan(child))
		}
	}
	return out
}

func cloneMeta(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
