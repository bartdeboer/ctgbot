package markdown

import "fmt"

func (d *Document) Chunk(limit int) ([]*Document, error) {
	if d == nil || d.Root == nil {
		return nil, fmt.Errorf("document is nil")
	}
	if err := d.Validate(); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 3500
	}
	chunks, err := chunkBlockSequence(d.Root.Children, limit)
	if err != nil {
		return nil, err
	}
	if len(chunks) == 0 {
		return []*Document{{Root: &Node{Kind: NodeDocument}}}, nil
	}
	out := make([]*Document, 0, len(chunks))
	for _, blocks := range chunks {
		root := &Node{Kind: NodeDocument, Children: blocks}
		if len(blocks) > 0 {
			root.Span = Span{Start: blocks[0].Span.Start, End: blocks[len(blocks)-1].Span.End}
		}
		doc := &Document{Root: root}
		if err := doc.Validate(); err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, nil
}

func chunkBlockSequence(blocks []*Node, limit int) ([][]*Node, error) {
	if len(blocks) == 0 {
		return nil, nil
	}
	var chunks [][]*Node
	var current []*Node
	currentSize := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		chunks = append(chunks, current)
		current = nil
		currentSize = 0
	}

	for _, block := range blocks {
		if block == nil {
			continue
		}
		parts, err := splitBlockNode(block, limit)
		if err != nil {
			return nil, err
		}
		for _, part := range parts {
			size := estimateBlockSize(part)
			joinCost := 0
			if len(current) > 0 {
				joinCost = 2
			}
			if len(current) > 0 && currentSize+joinCost+size <= limit {
				current = append(current, part)
				currentSize += joinCost + size
				continue
			}
			flush()
			current = []*Node{part}
			currentSize = size
		}
	}
	flush()
	return chunks, nil
}

func splitBlockNode(node *Node, limit int) ([]*Node, error) {
	if node == nil {
		return nil, nil
	}
	if estimateBlockSize(node) <= limit {
		return []*Node{Clone(node)}, nil
	}

	switch node.Kind {
	case NodeParagraph:
		return splitParagraphNode(node, limit)
	case NodeCodeBlock:
		return splitCodeBlockNode(node, limit), nil
	case NodeList:
		return splitListNode(node, limit)
	default:
		return nil, fmt.Errorf("unsupported block kind %q", node.Kind)
	}
}

func splitListNode(list *Node, limit int) ([]*Node, error) {
	if list == nil || list.Kind != NodeList {
		return nil, nil
	}
	var out []*Node
	var current []*Node
	currentSize := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		node := &Node{Kind: NodeList, Children: current, Span: list.Span, Meta: cloneMeta(list.Meta)}
		out = append(out, node)
		current = nil
		currentSize = 0
	}

	for _, item := range list.Children {
		parts, err := splitListItemNode(item, limit)
		if err != nil {
			return nil, err
		}
		for _, part := range parts {
			size := estimateListItemSize(part)
			joinCost := 0
			if len(current) > 0 {
				joinCost = 1
			}
			if len(current) > 0 && currentSize+joinCost+size <= limit {
				current = append(current, part)
				currentSize += joinCost + size
				continue
			}
			flush()
			current = []*Node{part}
			currentSize = size
		}
	}
	flush()
	return out, nil
}

func splitListItemNode(item *Node, limit int) ([]*Node, error) {
	if item == nil || item.Kind != NodeListItem {
		return nil, nil
	}
	if estimateListItemSize(item) <= limit {
		return []*Node{Clone(item)}, nil
	}
	parts, err := chunkBlockSequence(item.Children, maxInt(1, limit-2))
	if err != nil {
		return nil, err
	}
	out := make([]*Node, 0, len(parts))
	for _, blocks := range parts {
		out = append(out, &Node{Kind: NodeListItem, Children: blocks, Span: item.Span, Meta: cloneMeta(item.Meta)})
	}
	return out, nil
}

func splitParagraphNode(node *Node, limit int) ([]*Node, error) {
	parts := splitInlineSequence(node.Children, limit)
	out := make([]*Node, 0, len(parts))
	for _, children := range parts {
		para := &Node{Kind: NodeParagraph, Children: mergeInlineText(children), Span: node.Span, Meta: cloneMeta(node.Meta)}
		out = append(out, para)
	}
	return out, nil
}

func splitInlineSequence(nodes []*Node, limit int) [][]*Node {
	if len(nodes) == 0 {
		return nil
	}
	var chunks [][]*Node
	var current []*Node
	currentSize := 0

	flush := func() {
		if len(current) == 0 {
			return
		}
		chunks = append(chunks, current)
		current = nil
		currentSize = 0
	}

	for _, node := range nodes {
		parts := splitInlineNode(node, limit)
		for _, part := range parts {
			size := estimateInlineSize(part)
			if len(current) > 0 && currentSize+size <= limit {
				current = append(current, part)
				currentSize += size
				continue
			}
			flush()
			current = []*Node{part}
			currentSize = size
		}
	}
	flush()
	return chunks
}

func splitInlineNode(node *Node, limit int) []*Node {
	if node == nil {
		return nil
	}
	if estimateInlineSize(node) <= limit {
		return []*Node{Clone(node)}
	}
	if limit <= 0 {
		limit = 1
	}

	switch node.Kind {
	case NodeText, NodeInlineCode:
		return splitLeafInlineNode(node, limit)
	case NodeStrong, NodeEmphasis:
		parts := splitInlineSequence(node.Children, maxInt(1, limit))
		out := make([]*Node, 0, len(parts))
		for _, children := range parts {
			out = append(out, &Node{Kind: node.Kind, Children: mergeInlineText(children), Span: node.Span, Meta: cloneMeta(node.Meta)})
		}
		return out
	default:
		return []*Node{Clone(node)}
	}
}

func splitLeafInlineNode(node *Node, limit int) []*Node {
	if node == nil {
		return nil
	}
	text := node.Text
	if text == "" {
		return []*Node{Clone(node)}
	}
	var out []*Node
	for text != "" {
		part, rest := splitTextForLimit(text, limit)
		if part == "" {
			part, rest = forceSplit(text, maxInt(1, limit))
		}
		out = append(out, &Node{Kind: node.Kind, Text: part, Span: node.Span, Meta: cloneMeta(node.Meta)})
		text = rest
	}
	return out
}

func splitCodeBlockNode(node *Node, limit int) []*Node {
	if node == nil {
		return nil
	}
	text := node.Text
	if text == "" {
		return []*Node{Clone(node)}
	}
	lines := splitPreservingEmptyLines(text)
	var out []*Node
	current := ""
	flush := func() {
		if current == "" {
			return
		}
		out = append(out, &Node{Kind: NodeCodeBlock, Text: current, Span: node.Span, Meta: cloneMeta(node.Meta)})
		current = ""
	}
	for _, line := range lines {
		candidate := line
		if current != "" {
			candidate = current + "\n" + line
		}
		if current != "" && estimateCodeBlockSize(candidate) > limit {
			flush()
			candidate = line
		}
		if estimateCodeBlockSize(candidate) <= limit {
			current = candidate
			continue
		}
		remaining := line
		for remaining != "" {
			part, rest := splitTextForLimit(remaining, maxInt(1, limit))
			if part == "" {
				part, rest = forceSplit(remaining, maxInt(1, limit))
			}
			out = append(out, &Node{Kind: NodeCodeBlock, Text: part, Span: node.Span, Meta: cloneMeta(node.Meta)})
			remaining = rest
		}
	}
	flush()
	return out
}

func estimateBlockSize(node *Node) int {
	switch node.Kind {
	case NodeParagraph:
		return estimateInlineSequenceSize(node.Children)
	case NodeCodeBlock:
		return estimateCodeBlockSize(node.Text)
	case NodeList:
		total := 0
		for i, child := range node.Children {
			if i > 0 {
				total++
			}
			total += estimateListItemSize(child)
		}
		return total
	default:
		return 0
	}
}

func estimateListItemSize(node *Node) int {
	if node == nil {
		return 0
	}
	total := 0
	for i, child := range node.Children {
		if i > 0 {
			total += 2
		}
		size := estimateBlockSize(child)
		if child.Kind == NodeParagraph {
			size += 2
		}
		total += size
	}
	return total
}

func estimateInlineSequenceSize(nodes []*Node) int {
	total := 0
	for _, node := range nodes {
		total += estimateInlineSize(node)
	}
	return total
}

func estimateInlineSize(node *Node) int {
	if node == nil {
		return 0
	}
	switch node.Kind {
	case NodeText, NodeInlineCode:
		return textLen(node.Text)
	case NodeStrong, NodeEmphasis:
		return estimateInlineSequenceSize(node.Children)
	default:
		return 0
	}
}

func estimateCodeBlockSize(text string) int {
	return textLen(text)
}

func splitPreservingEmptyLines(text string) []string {
	if text == "" {
		return []string{""}
	}
	var out []string
	start := 0
	for i, r := range text {
		if r != '\n' {
			continue
		}
		out = append(out, text[start:i])
		start = i + 1
	}
	out = append(out, text[start:])
	return out
}

func cloneMeta(meta map[string]string) map[string]string {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		out[k] = v
	}
	return out
}
