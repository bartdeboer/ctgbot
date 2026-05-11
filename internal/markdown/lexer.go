package markdown

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
		if tok, ok := l.scanHeading(); ok {
			return tok
		}
		if tok, ok := l.scanListMarker(); ok {
			return tok
		}
	}
	start := l.position()
	switch {
	case l.match("**"):
		l.advanceN(2)
		return Token{Kind: TokenStrong, Text: "**", Span: Span{Start: start, End: l.position()}}
	case l.match("`"):
		return l.scanBackticks()
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
			if l.matchIndentedFenceMarkerLength() > 0 || l.matchHeadingMarkerLength() > 0 || l.matchListMarkerLength() > 0 {
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

func (l *Lexer) scanBackticks() Token {
	start := l.position()
	var b strings.Builder
	for !l.eof() && l.peek() == '`' {
		b.WriteRune(l.peek())
		l.advanceRune()
	}
	return Token{Kind: TokenBacktick, Text: b.String(), Span: Span{Start: start, End: l.position()}}
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
	startIdx := l.idx
	lineStart := l.position()
	indent := l.scanIndent()
	if indent > 3 || l.matchFenceMarkerLength() == 0 {
		l.idx = startIdx
		l.restorePosition(lineStart)
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
	return Token{Kind: TokenFence, Text: b.String(), Span: Span{Start: start, End: l.position()}, Indent: indent}, true
}

func (l *Lexer) scanHeading() (Token, bool) {
	startIdx := l.idx
	lineStart := l.position()
	indent := l.scanIndent()
	count := l.matchHeadingMarkerLength()
	if count == 0 {
		l.idx = startIdx
		l.restorePosition(lineStart)
		return Token{}, false
	}
	markerStart := l.position()
	for i := 0; i < count; i++ {
		l.advanceRune()
	}
	end := l.position()
	l.advanceRune() // consume required space
	return Token{Kind: TokenHeading, Text: strings.Repeat("#", count), Span: Span{Start: markerStart, End: end}, Indent: indent}, true
}

func (l *Lexer) scanListMarker() (Token, bool) {
	startIdx := l.idx
	lineStart := l.position()
	indent := l.scanIndent()
	length := l.matchListMarkerLength()
	if length == 0 {
		l.idx = startIdx
		l.restorePosition(lineStart)
		return Token{}, false
	}
	start := l.position()
	l.advanceN(length)
	return Token{Kind: TokenListMarker, Text: string(l.src[startIdx:l.idx]), Span: Span{Start: start, End: l.position()}, Indent: indent}, true
}

func (l *Lexer) scanIndent() int {
	indent := 0
	for !l.eof() {
		switch l.peek() {
		case ' ':
			indent++
			l.advanceRune()
		case '\t':
			indent += 4
			l.advanceRune()
		default:
			return indent
		}
	}
	return indent
}

func (l *Lexer) matchHeadingMarkerLength() int {
	count := 0
	for i := l.idx; i < len(l.src) && l.src[i] == '#'; i++ {
		count++
	}
	if count == 0 || count > 3 {
		return 0
	}
	if l.idx+count >= len(l.src) || l.src[l.idx+count] != ' ' {
		return 0
	}
	return count
}

func (l *Lexer) matchListMarkerLength() int {
	if l.eof() {
		return 0
	}
	if (l.peek() == '-' || l.peek() == '*') && l.idx+1 < len(l.src) && l.src[l.idx+1] == ' ' {
		return 2
	}
	j := l.idx
	for j < len(l.src) && l.src[j] >= '0' && l.src[j] <= '9' {
		j++
	}
	if j == l.idx || j+1 >= len(l.src) || l.src[j] != '.' || l.src[j+1] != ' ' {
		return 0
	}
	return j + 2 - l.idx
}

func (l *Lexer) matchFenceMarkerLength() int {
	count := 0
	for i := l.idx; i < len(l.src) && l.src[i] == '`'; i++ {
		count++
	}
	if count < 3 {
		return 0
	}
	return count
}

func (l *Lexer) matchIndentedFenceMarkerLength() int {
	clone := *l
	indent := clone.scanIndent()
	if indent > 3 {
		return 0
	}
	return clone.matchFenceMarkerLength()
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
