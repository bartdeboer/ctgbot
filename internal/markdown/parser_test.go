package markdown

import "testing"

func TestParseBuildsParagraphLinesAndSpans(t *testing.T) {
	doc, err := Parse("Hello **this** is a line.\nNext line.")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(doc.Blocks))
	}
	block := doc.Blocks[0]
	if block.Kind != ParagraphBlock {
		t.Fatalf("kind = %q, want paragraph", block.Kind)
	}
	if len(block.Lines) != 2 {
		t.Fatalf("lines len = %d, want 2", len(block.Lines))
	}
	line := block.Lines[0]
	if len(line.Spans) != 3 {
		t.Fatalf("spans len = %d, want 3", len(line.Spans))
	}
	if line.Spans[1].Kind != BoldSpan {
		t.Fatalf("span[1] kind = %q, want bold", line.Spans[1].Kind)
	}
}

func TestParseBuildsCodeBlockLines(t *testing.T) {
	doc, err := Parse("```go\nfmt.Println(\"hi\")\nfmt.Println(\"bye\")\n```")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(doc.Blocks))
	}
	block := doc.Blocks[0]
	if block.Kind != CodeBlock {
		t.Fatalf("kind = %q, want code_block", block.Kind)
	}
	if got := block.Meta["info"]; got != "go" {
		t.Fatalf("info = %q, want go", got)
	}
	if len(block.Lines) != 2 {
		t.Fatalf("lines len = %d, want 2", len(block.Lines))
	}
}

func TestDocumentGetLinesAndLineSlice(t *testing.T) {
	src := "first line\nsecond line\n\n```go\nthird line\nfourth line\n```"
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	lines := doc.GetLines()
	if len(lines) != 4 {
		t.Fatalf("lines len = %d, want 4", len(lines))
	}
	sliced := doc.LineSlice(Position{Offset: lines[1].StartPos.Offset}, Position{Offset: lines[2].EndPos.Offset})
	if len(sliced.Blocks) != 2 {
		t.Fatalf("sliced blocks len = %d, want 2", len(sliced.Blocks))
	}
	if len(sliced.Blocks[0].Lines) != 1 || len(sliced.Blocks[1].Lines) != 1 {
		t.Fatalf("sliced line counts = %d,%d want 1,1", len(sliced.Blocks[0].Lines), len(sliced.Blocks[1].Lines))
	}
}

func TestParseDetectsHeadingLines(t *testing.T) {
	doc, err := Parse("## Quick take\nNormal line")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	line := doc.Blocks[0].Lines[0]
	if line.HeadingLevel != 2 {
		t.Fatalf("heading level = %d, want 2", line.HeadingLevel)
	}
	if got := renderTextLine(line); got != "## Quick take" {
		t.Fatalf("rendered line = %q", got)
	}
}
