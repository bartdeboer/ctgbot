package markdownv2

import "strings"

type Parser struct {
	s   *Scanner
	cur Token
}

func Parse(src string) (*Document, error) {
	p := &Parser{s: NewScanner(src)}
	p.cur = p.s.Scan()
	return p.parseDocument(), nil
}

func (p *Parser) advance() {
	p.cur = p.s.Scan()
}

func (p *Parser) parseDocument() *Document {
	doc := &Document{}
	for p.cur.Kind != TokenEOF {
		switch p.cur.Kind {
		case TokenBlankLine:
			p.advance()
		case TokenFence:
			doc.Blocks = append(doc.Blocks, p.parseCodeBlock())
		case TokenListItem:
			doc.Blocks = append(doc.Blocks, p.parseList())
		default:
			doc.Blocks = append(doc.Blocks, p.parseParagraph())
		}
	}
	return doc
}

func (p *Parser) parseParagraph() Paragraph {
	start := p.cur.Span.Start
	var lines []string
	end := p.cur.Span.End
	for p.cur.Kind == TokenTextLine {
		lines = append(lines, strings.TrimRight(p.cur.Text, " \t"))
		end = p.cur.Span.End
		p.advance()
	}
	text := strings.TrimSpace(strings.Join(lines, "\n"))
	return Paragraph{
		Inlines: parseInlines(text),
		Span:    Span{Start: start, End: end},
	}
}

func (p *Parser) parseCodeBlock() CodeBlock {
	start := p.cur.Span.Start
	end := p.cur.Span.End
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
	return CodeBlock{
		Text: strings.Trim(strings.Join(lines, "\n"), "\n"),
		Span: Span{Start: start, End: end},
	}
}

func (p *Parser) parseList() List {
	start := p.cur.Span.Start
	end := p.cur.Span.End
	var items []ListItem
	for p.cur.Kind == TokenListItem {
		itemStart := p.cur.Span.Start
		itemEnd := p.cur.Span.End
		itemText := p.cur.Text
		p.advance()
		items = append(items, ListItem{
			Blocks: []Block{Paragraph{Inlines: parseInlines(itemText), Span: Span{Start: itemStart, End: itemEnd}}},
			Span:   Span{Start: itemStart, End: itemEnd},
		})
		end = itemEnd
		if p.cur.Kind == TokenBlankLine {
			break
		}
	}
	return List{Items: items, Span: Span{Start: start, End: end}}
}

func parseInlines(text string) []Inline {
	if text == "" {
		return nil
	}
	var out []Inline
	for len(text) > 0 {
		switch {
		case strings.HasPrefix(text, "**"):
			if inner, rest, ok := consumeDelimited(text, "**"); ok {
				out = append(out, Strong{Children: parseInlines(inner)})
				text = rest
				continue
			}
		case strings.HasPrefix(text, "*"):
			if inner, rest, ok := consumeDelimited(text, "*"); ok {
				out = append(out, Emphasis{Children: parseInlines(inner)})
				text = rest
				continue
			}
		case strings.HasPrefix(text, "`"):
			if inner, rest, ok := consumeDelimited(text, "`"); ok {
				out = append(out, InlineCode{Text: inner})
				text = rest
				continue
			}
		}
		next := nextMarker(text)
		out = append(out, Text{Value: text[:next]})
		text = text[next:]
	}
	return mergeInlines(out)
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

func mergeInlines(in []Inline) []Inline {
	out := make([]Inline, 0, len(in))
	for _, inline := range in {
		if txt, ok := inline.(Text); ok {
			if txt.Value == "" {
				continue
			}
			if len(out) > 0 {
				if prev, ok := out[len(out)-1].(Text); ok {
					out[len(out)-1] = Text{Value: prev.Value + txt.Value}
					continue
				}
			}
		}
		out = append(out, inline)
	}
	return out
}
