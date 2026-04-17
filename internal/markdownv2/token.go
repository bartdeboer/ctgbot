package markdownv2

type TokenKind string

const (
	TokenEOF       TokenKind = "eof"
	TokenBlankLine TokenKind = "blank_line"
	TokenTextLine  TokenKind = "text_line"
	TokenFence     TokenKind = "fence"
	TokenListItem  TokenKind = "list_item"
)

type Token struct {
	Kind TokenKind `json:"kind"`
	Text string    `json:"text"`
	Span Span      `json:"span"`
}
