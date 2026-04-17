package markdownv5

import "strings"

type Lexer struct {
	src       []rune
	idx       int
	line      int
	column    int
	lineStart bool
}

func NewLexer(src string) *Lexer {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	src = strings.ReplaceAll(src, "\r", "\n")
	return &Lexer{src: []rune(src), line: 1, column: 1, lineStart: true}
}

func (l *Lexer) Next() Token {
	if l.eof() {
		pos := l.position()
		return Token{Kind: TokenEOF, Span: Span{Start: pos, End: pos}}
	}
	if l.lineStart {
		if tok, ok := l.scanBlankLine(); ok {
			return tok
		}
		if tok, ok := l.scanFence(); ok {
			return tok
		}
	}
	start := l.position()
	switch {
	case l.match("**"):
		l.advanceN(2)
		return Token{Kind: TokenStrong, Text: "**", Span: Span{Start: start, End: l.position()}}
	case l.match("`"):
		l.advanceN(1)
		return Token{Kind: TokenBacktick, Text: "`", Span: Span{Start: start, End: l.position()}}
	case l.match("*"):
		l.advanceN(1)
		return Token{Kind: TokenEmphasis, Text: "*", Span: Span{Start: start, End: l.position()}}
	case l.peek() == '\n':
		l.advanceRune()
		return Token{Kind: TokenNewline, Text: "\n", Span: Span{Start: start, End: l.position()}}
	default:
		return l.scanText()
	}
}

func (l *Lexer) scanText() Token {
	start := l.position()
	var b strings.Builder
	for !l.eof() {
		if l.lineStart {
			if _, ok := l.scanBlankLinePreview(); ok {
				break
			}
			if l.match("```") {
				break
			}
		}
		if l.match("**") || l.match("`") || l.match("*") || l.peek() == '\n' {
			break
		}
		b.WriteRune(l.peek())
		l.advanceRune()
	}
	return Token{Kind: TokenText, Text: b.String(), Span: Span{Start: start, End: l.position()}}
}

func (l *Lexer) scanBlankLine() (Token, bool) {
	startIdx := l.idx
	start := l.position()
	for !l.eof() && (l.peek() == ' ' || l.peek() == '\t') {
		l.advanceRune()
	}
	if l.eof() || l.peek() != '\n' {
		l.idx = startIdx
		l.restorePosition(start)
		return Token{}, false
	}
	l.advanceRune()
	return Token{Kind: TokenBlankLine, Text: "\n", Span: Span{Start: start, End: l.position()}}, true
}

func (l *Lexer) scanBlankLinePreview() (Span, bool) {
	clone := *l
	tok, ok := clone.scanBlankLine()
	return tok.Span, ok
}

func (l *Lexer) scanFence() (Token, bool) {
	if !l.match("```") {
		return Token{}, false
	}
	start := l.position()
	var b strings.Builder
	for !l.eof() && l.peek() != '\n' {
		b.WriteRune(l.peek())
		l.advanceRune()
	}
	if !l.eof() && l.peek() == '\n' {
		l.advanceRune()
	}
	return Token{Kind: TokenFence, Text: b.String(), Span: Span{Start: start, End: l.position()}}, true
}

func (l *Lexer) eof() bool { return l.idx >= len(l.src) }
func (l *Lexer) peek() rune {
	if l.eof() {
		return 0
	}
	return l.src[l.idx]
}
func (l *Lexer) match(s string) bool {
	r := []rune(s)
	if l.idx+len(r) > len(l.src) {
		return false
	}
	for i := range r {
		if l.src[l.idx+i] != r[i] {
			return false
		}
	}
	return true
}
func (l *Lexer) advanceN(n int) {
	for range n {
		l.advanceRune()
	}
}
func (l *Lexer) advanceRune() {
	if l.eof() {
		return
	}
	if l.src[l.idx] == '\n' {
		l.idx++
		l.line++
		l.column = 1
		l.lineStart = true
		return
	}
	l.idx++
	l.column++
	l.lineStart = false
}
func (l *Lexer) position() Position {
	return Position{Offset: l.idx, Line: l.line, Column: l.column}
}
func (l *Lexer) restorePosition(pos Position) {
	l.line = pos.Line
	l.column = pos.Column
	l.lineStart = pos.Column == 1
}
