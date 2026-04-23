package markdown

import (
	"strconv"
	"strings"
)

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
	switch p.cur.Kind {
	case TokenFence:
		return p.parseCodeBlock()
	case TokenListMarker:
		return p.parseListBlock()
	}
	line, headingLevel := p.parseParagraphLineAndHeading()
	if headingLevel > 0 {
		block := &BlockNode{Kind: HeadingBlock, HeadingLevel: headingLevel, Lines: []*LineNode{line}}
		if line != nil {
			block.Span = Span{Start: line.StartPos, End: line.EndPos}
		}
		return block
	}
	return p.parseParagraphBlockFromFirstLine(line)
}

func (p *Parser) parseParagraphBlockFromFirstLine(first *LineNode) *BlockNode {
	block := &BlockNode{Kind: ParagraphBlock}
	if first != nil {
		block.Lines = append(block.Lines, first)
	}
	for p.cur.Kind != TokenEOF && p.cur.Kind != TokenBlankLine && p.cur.Kind != TokenFence && p.cur.Kind != TokenListMarker {
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
	line, _ := p.parseParagraphLineAndHeading()
	return line
}

func (p *Parser) parseParagraphLineAndHeading() (*LineNode, int) {
	if p.cur.Kind == TokenEOF || p.cur.Kind == TokenBlankLine || p.cur.Kind == TokenFence || p.cur.Kind == TokenListMarker {
		return nil, 0
	}
	start := p.cur.Span.Start
	headingLevel := 0
	if p.cur.Kind == TokenHeading {
		headingLevel = len([]rune(p.cur.Text))
		start = p.cur.Span.Start
		p.advance()
	}
	var tokens []Token
	end := start
	for p.cur.Kind != TokenEOF && p.cur.Kind != TokenBlankLine && p.cur.Kind != TokenNewline && p.cur.Kind != TokenFence && p.cur.Kind != TokenListMarker {
		tokens = append(tokens, p.cur)
		end = p.cur.Span.End
		p.advance()
	}
	if p.cur.Kind == TokenNewline {
		end = p.cur.Span.End
		p.advance()
	}
	return &LineNode{StartPos: start, EndPos: end, Spans: parseInlineTokens(tokens)}, headingLevel
}

func (p *Parser) parseListBlock() *BlockNode {
	block := &BlockNode{Kind: ListBlock}
	for p.cur.Kind == TokenListMarker {
		item := p.parseListItem()
		if item != nil {
			block.Items = append(block.Items, item)
		}
		if p.cur.Kind == TokenBlankLine {
			break
		}
	}
	if len(block.Items) > 0 {
		block.Span = Span{Start: block.Items[0].Span.Start, End: block.Items[len(block.Items)-1].Span.End}
	}
	return block
}

func (p *Parser) parseListItem() *BlockNode {
	if p.cur.Kind != TokenListMarker {
		return nil
	}
	markerTok := p.cur
	start := markerTok.Span.Start
	marker := strings.TrimSpace(markerTok.Text)
	ordered := isOrderedListMarker(marker)
	indent := markerTok.Indent
	p.advance()
	var tokens []Token
	end := start
	for p.cur.Kind != TokenEOF && p.cur.Kind != TokenBlankLine && p.cur.Kind != TokenNewline && p.cur.Kind != TokenFence && p.cur.Kind != TokenListMarker {
		tokens = append(tokens, p.cur)
		end = p.cur.Span.End
		p.advance()
	}
	if p.cur.Kind == TokenNewline {
		end = p.cur.Span.End
		p.advance()
	}
	lineStart := start
	if len(tokens) > 0 {
		lineStart = tokens[0].Span.Start
	}
	line := &LineNode{StartPos: lineStart, EndPos: end, Spans: parseInlineTokens(tokens)}
	item := &BlockNode{Kind: ListItemBlock, ListIndent: indent, Marker: marker, Ordered: ordered, Lines: []*LineNode{line}, Span: Span{Start: start, End: end}}
	return item
}

func isOrderedListMarker(marker string) bool {
	if !strings.HasSuffix(marker, ".") || len(marker) < 2 {
		return false
	}
	for _, r := range marker[:len(marker)-1] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func (p *Parser) parseCodeBlock() *BlockNode {
	startTok := p.cur
	fenceWidth := fenceTokenWidth(startTok)
	meta := map[string]string{}
	if info := fenceTokenInfo(startTok, fenceWidth); info != "" {
		meta["info"] = info
	}
	meta["fence"] = strconv.Itoa(fenceWidth)
	p.advance()
	block := &BlockNode{Kind: CodeBlock, Meta: meta}
	contentStart := startTok.Span.End
	for p.cur.Kind != TokenEOF && !isMatchingFenceToken(p.cur, fenceWidth) {
		lineStart := p.cur.Span.Start
		lineEnd := lineStart
		text := ""
		for p.cur.Kind != TokenEOF && !isMatchingFenceToken(p.cur, fenceWidth) && p.cur.Kind != TokenNewline {
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
	if isMatchingFenceToken(p.cur, fenceWidth) {
		blockEnd = p.cur.Span.End
		p.advance()
	}
	block.Span = Span{Start: blockStart, End: blockEnd}
	return block
}

func parseInlineTokens(tokens []Token) []*SpanNode {
	nodes, _, _ := parseInlineSeq(tokens, 0, "", "")
	return mergeTextSpans(nodes)
}

func parseInlineSeq(tokens []Token, idx int, stopKind TokenKind, stopText string) ([]*SpanNode, int, bool) {
	var nodes []*SpanNode
	for idx < len(tokens) {
		tok := tokens[idx]
		if stopKind != "" && tok.Kind == stopKind && (stopText == "" || tok.Text == stopText) {
			return nodes, idx + 1, true
		}
		switch tok.Kind {
		case TokenText, TokenListMarker:
			nodes = append(nodes, &SpanNode{Kind: TextSpan, Text: tok.Text, Span: tok.Span})
			idx++
		case TokenStrong:
			children, next, closed := parseInlineSeq(tokens, idx+1, TokenStrong, "")
			if closed {
				nodes = append(nodes, &SpanNode{Kind: BoldSpan, Children: mergeTextSpans(children), Span: Span{Start: tok.Span.Start, End: tokens[next-1].Span.End}})
			} else {
				nodes = append(nodes, &SpanNode{Kind: TextSpan, Text: tok.Text + rawSpanText(children), Span: Span{Start: tok.Span.Start, End: tok.Span.End}})
			}
			idx = next
		case TokenEmphasis:
			children, next, closed := parseInlineSeq(tokens, idx+1, TokenEmphasis, "")
			if closed {
				nodes = append(nodes, &SpanNode{Kind: ItalicSpan, Children: mergeTextSpans(children), Span: Span{Start: tok.Span.Start, End: tokens[next-1].Span.End}})
			} else {
				nodes = append(nodes, &SpanNode{Kind: TextSpan, Text: tok.Text + rawSpanText(children), Span: Span{Start: tok.Span.Start, End: tok.Span.End}})
			}
			idx = next
		case TokenBacktick:
			inner, next, closed, spanEnd := collectRawUntil(tokens, idx+1, TokenBacktick, tok.Text, tok.Span.End)
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

func collectRawUntil(tokens []Token, idx int, stopKind TokenKind, stopText string, fallbackEnd Position) (string, int, bool, Position) {
	text := ""
	end := fallbackEnd
	for idx < len(tokens) {
		if tokens[idx].Kind == stopKind && (stopText == "" || tokens[idx].Text == stopText) {
			return text, idx + 1, true, tokens[idx].Span.End
		}
		text += tokens[idx].Text
		end = tokens[idx].Span.End
		idx++
	}
	return text, idx, false, end
}

func fenceTokenWidth(tok Token) int {
	count := 0
	for _, r := range tok.Text {
		if r != '`' {
			break
		}
		count++
	}
	return count
}

func fenceTokenInfo(tok Token, width int) string {
	if width <= 0 {
		return ""
	}
	runes := []rune(tok.Text)
	if len(runes) <= width {
		return ""
	}
	return strings.TrimSpace(string(runes[width:]))
}

func isMatchingFenceToken(tok Token, width int) bool {
	return tok.Kind == TokenFence && fenceTokenWidth(tok) == width
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
