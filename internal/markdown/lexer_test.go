package markdown

import "testing"

func TestLexerPrefersStrongOverEmphasis(t *testing.T) {
	lx := NewLexer("**bold*")
	if tok := lx.Next(); tok.Kind != TokenStrong {
		t.Fatalf("first token kind = %q, want strong", tok.Kind)
	}
}

func TestLexerRecognizesFenceAtLineStart(t *testing.T) {
	lx := NewLexer("```go\ncode")
	if tok := lx.Next(); tok.Kind != TokenFence || tok.Text != "```go" {
		t.Fatalf("first token = %#v, want fence", tok)
	}
}

func TestLexerRecognizesListMarkerOnlyAtLineStart(t *testing.T) {
	lx := NewLexer("- item\ntext - not-list")
	if tok := lx.Next(); tok.Kind != TokenListMarker {
		t.Fatalf("first token kind = %q, want list_marker", tok.Kind)
	}
	for tok := lx.Next(); tok.Kind != TokenEOF; tok = lx.Next() {
		if tok.Kind == TokenListMarker {
			t.Fatalf("unexpected later list marker token %#v", tok)
		}
	}
}
