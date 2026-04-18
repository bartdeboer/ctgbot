package markdown

type TokenKind string

const (
	TokenEOF        TokenKind = "eof"
	TokenText       TokenKind = "text"
	TokenNewline    TokenKind = "newline"
	TokenBlankLine  TokenKind = "blank_line"
	TokenHeading    TokenKind = "heading"
	TokenListMarker TokenKind = "list_marker"
	TokenStrong     TokenKind = "strong"
	TokenEmphasis   TokenKind = "emphasis"
	TokenBacktick   TokenKind = "backtick"
	TokenFence      TokenKind = "fence"
)

type Token struct {
	Kind   TokenKind `json:"kind"`
	Text   string    `json:"text"`
	Span   Span      `json:"span"`
	Indent int       `json:"indent,omitempty"`
}
