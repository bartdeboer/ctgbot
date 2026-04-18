package markdown

import "testing"

func TestLexerScansHeadingToken(t *testing.T) {
	lx := NewLexer("## Heading\n")
	tok := lx.Next()
	if tok.Kind != TokenHeading {
		t.Fatalf("kind = %q, want heading", tok.Kind)
	}
	if tok.Text != "##" {
		t.Fatalf("text = %q, want ##", tok.Text)
	}
	next := lx.Next()
	if next.Kind != TokenText || next.Text != "Heading" {
		t.Fatalf("next = %#v, want text token", next)
	}
}
