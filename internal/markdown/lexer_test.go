package markdown

import "testing"

func TestLexerScansHeadingToken(t *testing.T) {
	lx := NewLexer("  ## Heading\n")
	tok := lx.Next()
	if tok.Kind != TokenHeading {
		t.Fatalf("kind = %q, want heading", tok.Kind)
	}
	if tok.Text != "##" {
		t.Fatalf("text = %q, want ##", tok.Text)
	}
	if tok.Indent != 2 {
		t.Fatalf("indent = %d, want 2", tok.Indent)
	}
	next := lx.Next()
	if next.Kind != TokenText || next.Text != "Heading" {
		t.Fatalf("next = %#v, want text token", next)
	}
}

func TestLexerScansUnorderedListMarker(t *testing.T) {
	lx := NewLexer("  - item\n")
	tok := lx.Next()
	if tok.Kind != TokenListMarker {
		t.Fatalf("kind = %q, want list_marker", tok.Kind)
	}
	if tok.Text != "  - " {
		t.Fatalf("text = %q, want %q", tok.Text, "  - ")
	}
	if tok.Indent != 2 {
		t.Fatalf("indent = %d, want 2", tok.Indent)
	}
	next := lx.Next()
	if next.Kind != TokenText || next.Text != "item" {
		t.Fatalf("next = %#v, want text token", next)
	}
}

func TestLexerScansOrderedListMarker(t *testing.T) {
	lx := NewLexer("    12. item\n")
	tok := lx.Next()
	if tok.Kind != TokenListMarker {
		t.Fatalf("kind = %q, want list_marker", tok.Kind)
	}
	if tok.Text != "    12. " {
		t.Fatalf("text = %q, want %q", tok.Text, "    12. ")
	}
	if tok.Indent != 4 {
		t.Fatalf("indent = %d, want 4", tok.Indent)
	}
	next := lx.Next()
	if next.Kind != TokenText || next.Text != "item" {
		t.Fatalf("next = %#v, want text token", next)
	}
}

func TestLexerDoesNotTokenizeInlineHyphenAsListMarker(t *testing.T) {
	lx := NewLexer("photo-1408.jpg\n")
	tok := lx.Next()
	if tok.Kind != TokenText || tok.Text != "photo-1408.jpg" {
		t.Fatalf("tok = %#v, want plain text token", tok)
	}
}

func TestLexerDoesNotTokenizeDecimalAsOrderedListMarker(t *testing.T) {
	lx := NewLexer("1.23 is a number\n")
	tok := lx.Next()
	if tok.Kind != TokenText || tok.Text != "1.23 is a number" {
		t.Fatalf("tok = %#v, want plain text token", tok)
	}
}

func TestLexerScansVariableBacktickRun(t *testing.T) {
	lx := NewLexer("``code")
	tok := lx.Next()
	if tok.Kind != TokenBacktick {
		t.Fatalf("kind = %q, want backtick", tok.Kind)
	}
	if tok.Text != "``" {
		t.Fatalf("text = %q, want %q", tok.Text, "``")
	}
	if tok.Span.Length() != 2 {
		t.Fatalf("span length = %d, want 2", tok.Span.Length())
	}
}

func TestLexerScansVariableFenceToken(t *testing.T) {
	lx := NewLexer("````go\nbody\n````\n")
	tok := lx.Next()
	if tok.Kind != TokenFence {
		t.Fatalf("kind = %q, want fence", tok.Kind)
	}
	if tok.Text != "````go" {
		t.Fatalf("text = %q, want %q", tok.Text, "````go")
	}
}
