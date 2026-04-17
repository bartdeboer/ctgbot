package markdown

import "strings"

type Parser struct {
	lx  *Lexer
	cur Token
}

func Parse(src string) (*Document, error) {
	p := &Parser{lx: NewLexer(src)}
	p.cur = p.lx.Next()
	return p.parseDocument(), nil
}

func (p *Parser) advance() { p.cur = p.lx.Next() }

func (p *Parser) parseDocument() *Document {
	doc := &Document{}
	for p.cur.Kind != TokenEOF {
		if p.cur.Kind == TokenBlankLine || p.cur.Kind == TokenNewline {
			p.advance()
			continue
		}
		doc.Blocks = append(doc.Blocks, p.parseBlock())
	}
	if len(doc.Blocks) > 0 {
		doc.Span = Span{Start: doc.Blocks[0].Span.Start, End: doc.Blocks[len(doc.Blocks)-1].Span.End}
	}
	return doc
}

func (p *Parser) parseBlock() *BlockNode {
	if p.cur.Kind == TokenFence {
		return p.parseCodeBlock()
	}
	return p.parseParagraphBlock()
}

func (p *Parser) parseParagraphBlock() *BlockNode {
	block := &BlockNode{Kind: ParagraphBlock}
	for p.cur.Kind != TokenEOF && p.cur.Kind != TokenBlankLine && p.cur.Kind != TokenFence {
		line := p.parseParagraphLine()
		if line != nil {
			block.Lines = append(block.Lines, line)
		}
		if p.cur.Kind == TokenBlankLine {
			break
		}
	}
	if len(block.Lines) > 0 {
		block.Span = Span{Start: block.Lines[0].StartPos, End: block.Lines[len(block.Lines)-1].EndPos}
	}
	return block
}

func (p *Parser) parseParagraphLine() *LineNode {
	if p.cur.Kind == TokenEOF || p.cur.Kind == TokenBlankLine || p.cur.Kind == TokenFence {
		return nil
	}
	start := p.cur.Span.Start
	var tokens []Token
	end := start
	for p.cur.Kind != TokenEOF && p.cur.Kind != TokenBlankLine && p.cur.Kind != TokenNewline && p.cur.Kind != TokenFence {
		tokens = append(tokens, p.cur)
		end = p.cur.Span.End
		p.advance()
	}
	if p.cur.Kind == TokenNewline {
		end = p.cur.Span.End
		p.advance()
	}
	headingLevel, tokens := detectHeadingTokens(tokens)
	return &LineNode{StartPos: start, EndPos: end, HeadingLevel: headingLevel, Spans: parseInlineTokens(tokens)}
}

func detectHeadingTokens(tokens []Token) (int, []Token) {
	if len(tokens) == 0 || tokens[0].Kind != TokenText {
		return 0, tokens
	}
	text := tokens[0].Text
	level := 0
	rest := ""
	switch {
	case strings.HasPrefix(text, "### "):
		level, rest = 3, text[4:]
	case strings.HasPrefix(text, "## "):
		level, rest = 2, text[3:]
	case strings.HasPrefix(text, "# "):
		level, rest = 1, text[2:]
	default:
		return 0, tokens
	}
	cloned := append([]Token(nil), tokens...)
	cloned[0].Text = rest
	return level, cloned
}

func (p *Parser) parseCodeBlock() *BlockNode {
	startTok := p.cur
	meta := map[string]string{}
	if len(startTok.Text) > 3 {
		meta["info"] = startTok.Text[3:]
	}
	p.advance()
	block := &BlockNode{Kind: CodeBlock, Meta: meta}
	contentStart := startTok.Span.End
	for p.cur.Kind != TokenEOF && p.cur.Kind != TokenFence {
		lineStart := p.cur.Span.Start
		lineEnd := lineStart
		text := ""
		for p.cur.Kind != TokenEOF && p.cur.Kind != TokenFence && p.cur.Kind != TokenNewline {
			text += p.cur.Text
			lineEnd = p.cur.Span.End
			p.advance()
		}
		if p.cur.Kind == TokenNewline {
			lineEnd = p.cur.Span.End
			p.advance()
		}
		block.Lines = append(block.Lines, &LineNode{
			StartPos: lineStart,
			EndPos:   lineEnd,
			Spans:    []*SpanNode{{Kind: TextSpan, Text: text, Span: Span{Start: lineStart, End: lineEnd}}},
		})
	}
	blockStart := startTok.Span.Start
	blockEnd := contentStart
	if len(block.Lines) > 0 {
		blockEnd = block.Lines[len(block.Lines)-1].EndPos
	}
	if p.cur.Kind == TokenFence {
		blockEnd = p.cur.Span.End
		p.advance()
	}
	block.Span = Span{Start: blockStart, End: blockEnd}
	return block
}

func parseInlineTokens(tokens []Token) []*SpanNode {
	nodes, _, _ := parseInlineSeq(tokens, 0, "")
	return mergeTextSpans(nodes)
}

func parseInlineSeq(tokens []Token, idx int, stop TokenKind) ([]*SpanNode, int, bool) {
	var nodes []*SpanNode
	for idx < len(tokens) {
		tok := tokens[idx]
		if stop != "" && tok.Kind == stop {
			return nodes, idx + 1, true
		}
		switch tok.Kind {
		case TokenText:
			nodes = append(nodes, &SpanNode{Kind: TextSpan, Text: tok.Text, Span: tok.Span})
			idx++
		case TokenStrong:
			children, next, closed := parseInlineSeq(tokens, idx+1, TokenStrong)
			if closed {
				nodes = append(nodes, &SpanNode{Kind: BoldSpan, Children: mergeTextSpans(children), Span: Span{Start: tok.Span.Start, End: tokens[next-1].Span.End}})
			} else {
				nodes = append(nodes, &SpanNode{Kind: TextSpan, Text: tok.Text + rawSpanText(children), Span: Span{Start: tok.Span.Start, End: tok.Span.End}})
			}
			idx = next
		case TokenEmphasis:
			children, next, closed := parseInlineSeq(tokens, idx+1, TokenEmphasis)
			if closed {
				nodes = append(nodes, &SpanNode{Kind: ItalicSpan, Children: mergeTextSpans(children), Span: Span{Start: tok.Span.Start, End: tokens[next-1].Span.End}})
			} else {
				nodes = append(nodes, &SpanNode{Kind: TextSpan, Text: tok.Text + rawSpanText(children), Span: Span{Start: tok.Span.Start, End: tok.Span.End}})
			}
			idx = next
		case TokenBacktick:
			inner, next, closed, spanEnd := collectRawUntil(tokens, idx+1, TokenBacktick, tok.Span.End)
			if closed {
				nodes = append(nodes, &SpanNode{Kind: InlineCodeSpan, Text: inner, Span: Span{Start: tok.Span.Start, End: spanEnd}})
			} else {
				nodes = append(nodes, &SpanNode{Kind: TextSpan, Text: tok.Text + inner, Span: tok.Span})
			}
			idx = next
		default:
			nodes = append(nodes, &SpanNode{Kind: TextSpan, Text: tok.Text, Span: tok.Span})
			idx++
		}
	}
	return nodes, idx, false
}

func collectRawUntil(tokens []Token, idx int, stop TokenKind, fallbackEnd Position) (string, int, bool, Position) {
	text := ""
	end := fallbackEnd
	for idx < len(tokens) {
		if tokens[idx].Kind == stop {
			return text, idx + 1, true, tokens[idx].Span.End
		}
		text += tokens[idx].Text
		end = tokens[idx].Span.End
		idx++
	}
	return text, idx, false, end
}

func mergeTextSpans(in []*SpanNode) []*SpanNode {
	out := make([]*SpanNode, 0, len(in))
	for _, node := range in {
		if node == nil {
			continue
		}
		if node.Kind == TextSpan && node.Text == "" {
			continue
		}
		if len(out) > 0 && out[len(out)-1].Kind == TextSpan && node.Kind == TextSpan {
			out[len(out)-1].Text += node.Text
			out[len(out)-1].Span.End = node.Span.End
			continue
		}
		out = append(out, node)
	}
	return out
}

func rawSpanText(nodes []*SpanNode) string {
	text := ""
	for _, node := range nodes {
		switch node.Kind {
		case TextSpan, InlineCodeSpan:
			text += node.Text
		case BoldSpan:
			text += "**" + rawSpanText(node.Children)
		case ItalicSpan:
			text += "*" + rawSpanText(node.Children)
		default:
			text += rawSpanText(node.Children)
		}
	}
	return text
}
