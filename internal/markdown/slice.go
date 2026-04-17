package markdown

import "strconv"

func (d *Document) Slice(start, end Position) *Document {
	if d == nil || d.Root == nil {
		return &Document{Root: &Node{Kind: NodeDocument}}
	}
	if end.Offset < start.Offset {
		start, end = end, start
	}
	if start.Offset == end.Offset {
		return &Document{Root: &Node{Kind: NodeDocument}}
	}
	var children []*Node
	for _, child := range d.Root.Children {
		if sliced := sliceNode(child, start, end); sliced != nil {
			children = append(children, sliced)
		}
	}
	root := &Node{Kind: NodeDocument, Children: children}
	if len(children) > 0 {
		root.Span = Span{Start: children[0].Span.Start, End: children[len(children)-1].Span.End}
	}
	return &Document{Root: root}
}

func sliceNode(node *Node, start, end Position) *Node {
	if node == nil || !spansOverlap(node.Span, start, end) {
		return nil
	}

	switch node.Kind {
	case NodeDocument, NodeParagraph, NodeList, NodeListItem, NodeStrong, NodeEmphasis:
		var children []*Node
		for _, child := range node.Children {
			if sliced := sliceNode(child, start, end); sliced != nil {
				children = append(children, sliced)
			}
		}
		if len(children) == 0 {
			return nil
		}
		out := &Node{Kind: node.Kind, Children: children, Meta: cloneMeta(node.Meta)}
		out.Span = Span{Start: children[0].Span.Start, End: children[len(children)-1].Span.End}
		return out
	case NodeText, NodeInlineCode, NodeCodeBlock:
		return sliceLeafNode(node, start, end)
	default:
		return nil
	}
}

func sliceLeafNode(node *Node, start, end Position) *Node {
	content := contentSpan(node)
	if !spansOverlap(content, start, end) {
		return nil
	}
	runes := []rune(node.Text)
	contentLen := len(runes)
	if contentLen == 0 {
		return Clone(node)
	}
	from := clampInt(start.Offset-content.Start.Offset, 0, contentLen)
	to := clampInt(end.Offset-content.Start.Offset, 0, contentLen)
	if to <= from {
		if start.Offset <= content.Start.Offset && end.Offset >= content.End.Offset {
			return Clone(node)
		}
		return nil
	}
	out := &Node{
		Kind: node.Kind,
		Text: string(runes[from:to]),
		Span: Span{
			Start: offsetPosition(content.Start, from),
			End:   offsetPosition(content.Start, to),
		},
		Meta: cloneMeta(node.Meta),
	}
	return out
}

func contentSpan(node *Node) Span {
	if node == nil {
		return Span{}
	}
	start, okStart := metaPosition(node.Meta, "content_start")
	end, okEnd := metaPosition(node.Meta, "content_end")
	if okStart && okEnd {
		return Span{Start: start, End: end}
	}
	return node.Span
}

func metaPosition(meta map[string]string, prefix string) (Position, bool) {
	if len(meta) == 0 {
		return Position{}, false
	}
	offset, err1 := strconv.Atoi(meta[prefix+"_offset"])
	line, err2 := strconv.Atoi(meta[prefix+"_line"])
	col, err3 := strconv.Atoi(meta[prefix+"_column"])
	if err1 != nil || err2 != nil || err3 != nil {
		return Position{}, false
	}
	return Position{Offset: offset, Line: line, Column: col}, true
}

func spansOverlap(span Span, start, end Position) bool {
	return span.End.Offset > start.Offset && span.Start.Offset < end.Offset
}

func offsetPosition(base Position, delta int) Position {
	return Position{Offset: base.Offset + delta, Line: base.Line, Column: base.Column + delta}
}

func clampInt(v, minV, maxV int) int {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}
