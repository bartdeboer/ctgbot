package markdownv3

import "strings"

type Scanner struct {
	lines  []string
	offset int
	line   int
}

func NewScanner(src string) *Scanner {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	return &Scanner{lines: strings.Split(src, "\n"), line: 1}
}

func (s *Scanner) Scan() Token {
	if s.line > len(s.lines) {
		pos := s.position(0)
		return Token{Kind: TokenEOF, Span: Span{Start: pos, End: pos}}
	}
	raw := s.lines[s.line-1]
	start := s.position(0)
	end := s.position(len(raw))
	span := Span{Start: start, End: end}
	s.advance(raw)

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return Token{Kind: TokenBlankLine, Span: span}
	}
	if strings.HasPrefix(trimmed, "```") {
		return Token{Kind: TokenFence, Text: trimmed, Span: span}
	}
	if strings.HasPrefix(trimmed, "- ") || strings.HasPrefix(trimmed, "* ") {
		return Token{Kind: TokenListItem, Text: strings.TrimSpace(trimmed[2:]), Span: span}
	}
	return Token{Kind: TokenTextLine, Text: raw, Span: span}
}

func (s *Scanner) advance(line string) {
	s.offset += len(line)
	if s.line < len(s.lines)+1 {
		s.offset++
	}
	s.line++
}

func (s *Scanner) position(column int) Position {
	return Position{Offset: s.offset + column, Line: s.line, Column: column + 1}
}
