package markdownv3

import "strings"

type Parser struct {
	s   *Scanner
	cur Token
}

func Parse(src string) (*Document, error) {
	p := &Parser{s: NewScanner(src)}
	p.cur = p.s.Scan()
	doc := p.parseDocument()
	return doc, doc.Validate()
}

func (p *Parser) advance() {
	p.cur = p.s.Scan()
}

func (p *Parser) parseDocument() *Document {
	root := &Node{Kind: NodeDocument}
	for p.cur.Kind != TokenEOF {
		switch p.cur.Kind {
		case TokenBlankLine:
			p.advance()
		case TokenFence:
			root.Children = append(root.Children, p.parseCodeBlock())
		case TokenListItem:
			root.Children = append(root.Children, p.parseList())
		default:
			root.Children = append(root.Children, p.parseParagraph())
		}
	}
	if len(root.Children) > 0 {
		root.Span = Span{Start: root.Children[0].Span.Start, End: root.Children[len(root.Children)-1].Span.End}
	}
	return &Document{Root: root}
}

func (p *Parser) parseParagraph() *Node {
	start := p.cur.Span.Start
	var lines []string
	end := p.cur.Span.End
	for p.cur.Kind == TokenTextLine {
		lines = append(lines, strings.TrimRight(p.cur.Text, " \t"))
		end = p.cur.Span.End
		p.advance()
	}
	return &Node{Kind: NodeParagraph, Children: parseInlines(strings.TrimSpace(strings.Join(lines, "\n"))), Span: Span{Start: start, End: end}}
}

func (p *Parser) parseCodeBlock() *Node {
	start := p.cur.Span.Start
	end := p.cur.Span.End
	meta := map[string]string{}
	if fence := strings.TrimSpace(p.cur.Text); len(fence) > 3 {
		meta["info"] = strings.TrimSpace(fence[3:])
	}
	p.advance()
	var lines []string
	for p.cur.Kind != TokenEOF && p.cur.Kind != TokenFence {
		if p.cur.Kind == TokenBlankLine {
			lines = append(lines, "")
		} else {
			lines = append(lines, p.cur.Text)
		}
		end = p.cur.Span.End
		p.advance()
	}
	if p.cur.Kind == TokenFence {
		end = p.cur.Span.End
		p.advance()
	}
	node := &Node{Kind: NodeCodeBlock, Text: strings.Trim(strings.Join(lines, "\n"), "\n"), Span: Span{Start: start, End: end}}
	if len(meta) > 0 {
		node.Meta = meta
	}
	return node
}

func (p *Parser) parseList() *Node {
	start := p.cur.Span.Start
	list := &Node{Kind: NodeList, Meta: map[string]string{"ordered": "false"}}
	for p.cur.Kind == TokenListItem {
		itemStart := p.cur.Span.Start
		itemEnd := p.cur.Span.End
		text := p.cur.Text
		p.advance()
		item := &Node{Kind: NodeListItem, Span: Span{Start: itemStart, End: itemEnd}}
		item.Children = append(item.Children, &Node{Kind: NodeParagraph, Children: parseInlines(text), Span: Span{Start: itemStart, End: itemEnd}})
		list.Children = append(list.Children, item)
		if p.cur.Kind == TokenBlankLine {
			break
		}
	}
	list.Span = Span{Start: start, End: list.Children[len(list.Children)-1].Span.End}
	return list
}

func parseInlines(text string) []*Node {
	if text == "" {
		return nil
	}
	var out []*Node
	for len(text) > 0 {
		switch {
		case strings.HasPrefix(text, "**"):
			if inner, rest, ok := consumeDelimited(text, "**"); ok {
				out = append(out, &Node{Kind: NodeStrong, Children: parseInlines(inner)})
				text = rest
				continue
			}
		case strings.HasPrefix(text, "*"):
			if inner, rest, ok := consumeDelimited(text, "*"); ok {
				out = append(out, &Node{Kind: NodeEmphasis, Children: parseInlines(inner)})
				text = rest
				continue
			}
		case strings.HasPrefix(text, "`"):
			if inner, rest, ok := consumeDelimited(text, "`"); ok {
				out = append(out, &Node{Kind: NodeInlineCode, Text: inner})
				text = rest
				continue
			}
		}
		next := nextMarker(text)
		out = append(out, &Node{Kind: NodeText, Text: text[:next]})
		text = text[next:]
	}
	return mergeInlineText(out)
}

func consumeDelimited(text string, marker string) (inner string, rest string, ok bool) {
	if !strings.HasPrefix(text, marker) {
		return "", text, false
	}
	body := text[len(marker):]
	idx := strings.Index(body, marker)
	if marker == "**" {
		idx = strings.LastIndex(body, marker)
	}
	if idx < 0 {
		return "", text, false
	}
	return body[:idx], body[idx+len(marker):], true
}

func nextMarker(text string) int {
	min := len(text)
	for _, marker := range []string{"**", "*", "`"} {
		if idx := strings.Index(text, marker); idx >= 0 && idx < min {
			min = idx
		}
	}
	if min == 0 {
		return len(text)
	}
	return min
}

func mergeInlineText(in []*Node) []*Node {
	out := make([]*Node, 0, len(in))
	for _, node := range in {
		if node == nil {
			continue
		}
		if node.Kind == NodeText && node.Text == "" {
			continue
		}
		if len(out) > 0 && out[len(out)-1].Kind == NodeText && node.Kind == NodeText {
			out[len(out)-1].Text += node.Text
			continue
		}
		out = append(out, node)
	}
	return out
}
