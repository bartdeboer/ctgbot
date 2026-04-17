package markdown

type Parser struct {
	lx  *Lexer
	cur Token
}

func Parse(src string) (*Document, error) {
	p := &Parser{lx: NewLexer(src)}
	p.cur = p.lx.Next()
	doc := p.parseDocument()
	return doc, doc.Validate()
}

func (p *Parser) advance() { p.cur = p.lx.Next() }

func (p *Parser) parseDocument() *Document {
	root := &Node{Kind: NodeDocument}
	for p.cur.Kind != TokenEOF {
		if p.cur.Kind == TokenBlankLine || p.cur.Kind == TokenNewline {
			p.advance()
			continue
		}
		root.Children = append(root.Children, p.parseBlock())
	}
	if len(root.Children) > 0 {
		root.Span = Span{Start: root.Children[0].Span.Start, End: root.Children[len(root.Children)-1].Span.End}
	}
	return &Document{Root: root}
}

func (p *Parser) parseBlock() *Node {
	switch p.cur.Kind {
	case TokenFence:
		return p.parseCodeBlock()
	case TokenListMarker:
		return p.parseList()
	default:
		return p.parseParagraph()
	}
}

func (p *Parser) parseCodeBlock() *Node {
	startTok := p.cur
	meta := map[string]string{}
	if len(startTok.Text) > 3 {
		meta["info"] = startTok.Text[3:]
	}
	p.advance()
	textStart := startTok.Span.End
	textEnd := textStart
	code := ""
	for p.cur.Kind != TokenEOF && p.cur.Kind != TokenFence {
		code += p.cur.Text
		textEnd = p.cur.Span.End
		p.advance()
	}
	end := textEnd
	if p.cur.Kind == TokenFence {
		end = p.cur.Span.End
		p.advance()
	}
	node := &Node{Kind: NodeCodeBlock, Text: trimTrailingNewline(code), Span: Span{Start: startTok.Span.Start, End: end}}
	if len(meta) > 0 {
		node.Meta = meta
	}
	_ = textStart
	return node
}

func (p *Parser) parseList() *Node {
	start := p.cur.Span.Start
	list := &Node{Kind: NodeList, Meta: map[string]string{"ordered": "false"}}
	for p.cur.Kind == TokenListMarker {
		itemStart := p.cur.Span.Start
		p.advance()
		inlineTokens := p.collectInlineUntilLineEnd()
		paragraph := &Node{Kind: NodeParagraph, Children: parseInlineTokens(inlineTokens), Span: spanForInlineChildren(itemStart, inlineTokens)}
		item := &Node{Kind: NodeListItem, Children: []*Node{paragraph}, Span: Span{Start: itemStart, End: paragraph.Span.End}}
		list.Children = append(list.Children, item)
		if p.cur.Kind == TokenBlankLine {
			break
		}
	}
	list.Span = Span{Start: start, End: list.Children[len(list.Children)-1].Span.End}
	return list
}

func (p *Parser) parseParagraph() *Node {
	start := p.cur.Span.Start
	var inlineTokens []Token
	end := p.cur.Span.End
	for p.cur.Kind != TokenEOF {
		if p.cur.Kind == TokenBlankLine || p.cur.Kind == TokenFence || p.cur.Kind == TokenListMarker {
			break
		}
		inlineTokens = append(inlineTokens, p.cur)
		end = p.cur.Span.End
		p.advance()
	}
	children := parseInlineTokens(inlineTokens)
	return &Node{Kind: NodeParagraph, Children: children, Span: Span{Start: start, End: end}}
}

func (p *Parser) collectInlineUntilLineEnd() []Token {
	var out []Token
	for p.cur.Kind != TokenEOF && p.cur.Kind != TokenBlankLine && p.cur.Kind != TokenNewline {
		out = append(out, p.cur)
		p.advance()
	}
	if p.cur.Kind == TokenNewline {
		p.advance()
	}
	return out
}

func parseInlineTokens(tokens []Token) []*Node {
	nodes, _, _ := parseInlineSeq(tokens, 0, "")
	return mergeInlineText(nodes)
}

func parseInlineSeq(tokens []Token, idx int, stop TokenKind) ([]*Node, int, bool) {
	var nodes []*Node
	for idx < len(tokens) {
		tok := tokens[idx]
		if stop != "" && tok.Kind == stop {
			return nodes, idx + 1, true
		}
		switch tok.Kind {
		case TokenText:
			nodes = append(nodes, &Node{Kind: NodeText, Text: tok.Text, Span: tok.Span})
			idx++
		case TokenNewline:
			nodes = append(nodes, &Node{Kind: NodeText, Text: "\n", Span: tok.Span})
			idx++
		case TokenStrong:
			children, next, closed := parseInlineSeq(tokens, idx+1, TokenStrong)
			if closed {
				nodes = append(nodes, &Node{Kind: NodeStrong, Children: mergeInlineText(children), Span: Span{Start: tok.Span.Start, End: tokens[next-1].Span.End}})
			} else {
				nodes = append(nodes, &Node{Kind: NodeText, Text: tok.Text + rawInlineText(children), Span: spanFrom(tok.Span.Start, children, tok.Span.End)})
			}
			idx = next
		case TokenEmphasis:
			children, next, closed := parseInlineSeq(tokens, idx+1, TokenEmphasis)
			if closed {
				nodes = append(nodes, &Node{Kind: NodeEmphasis, Children: mergeInlineText(children), Span: Span{Start: tok.Span.Start, End: tokens[next-1].Span.End}})
			} else {
				nodes = append(nodes, &Node{Kind: NodeText, Text: tok.Text + rawInlineText(children), Span: spanFrom(tok.Span.Start, children, tok.Span.End)})
			}
			idx = next
		case TokenBacktick:
			inner, next, closed := collectRawUntil(tokens, idx+1, TokenBacktick)
			if closed {
				nodes = append(nodes, &Node{Kind: NodeInlineCode, Text: inner, Span: Span{Start: tok.Span.Start, End: tokens[next-1].Span.End}})
			} else {
				nodes = append(nodes, &Node{Kind: NodeText, Text: tok.Text + inner, Span: tok.Span})
			}
			idx = next
		default:
			nodes = append(nodes, &Node{Kind: NodeText, Text: tok.Text, Span: tok.Span})
			idx++
		}
	}
	return nodes, idx, false
}

func collectRawUntil(tokens []Token, idx int, stop TokenKind) (string, int, bool) {
	text := ""
	for idx < len(tokens) {
		if tokens[idx].Kind == stop {
			return text, idx + 1, true
		}
		text += tokens[idx].Text
		idx++
	}
	return text, idx, false
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
			out[len(out)-1].Span.End = node.Span.End
			continue
		}
		out = append(out, node)
	}
	return out
}

func rawInlineText(nodes []*Node) string {
	text := ""
	for _, node := range nodes {
		switch node.Kind {
		case NodeText, NodeInlineCode:
			text += node.Text
		case NodeStrong:
			text += "**" + rawInlineText(node.Children)
		case NodeEmphasis:
			text += "*" + rawInlineText(node.Children)
		default:
			text += rawInlineText(node.Children)
		}
	}
	return text
}

func spanForInlineChildren(start Position, tokens []Token) Span {
	if len(tokens) == 0 {
		return Span{Start: start, End: start}
	}
	return Span{Start: start, End: tokens[len(tokens)-1].Span.End}
}

func spanFrom(start Position, children []*Node, fallbackEnd Position) Span {
	if len(children) == 0 {
		return Span{Start: start, End: fallbackEnd}
	}
	return Span{Start: start, End: children[len(children)-1].Span.End}
}

func trimTrailingNewline(s string) string {
	for len(s) > 0 && s[len(s)-1] == '\n' {
		s = s[:len(s)-1]
	}
	return s
}
