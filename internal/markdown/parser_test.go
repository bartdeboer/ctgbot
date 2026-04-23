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

func TestParseBuildsInlineCodeWithMatchingBacktickWidth(t *testing.T) {
	doc, err := Parse("Use ``a`b`` here.")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	block := doc.Blocks[0]
	line := block.Lines[0]
	if len(line.Spans) < 3 {
		t.Fatalf("spans len = %d, want at least 3", len(line.Spans))
	}
	if line.Spans[1].Kind != InlineCodeSpan {
		t.Fatalf("span[1] kind = %q, want inline_code", line.Spans[1].Kind)
	}
	if line.Spans[1].Text != "a`b" {
		t.Fatalf("inline code = %q, want %q", line.Spans[1].Text, "a`b")
	}
}

func TestParseBuildsCodeBlockWithMatchingFenceWidth(t *testing.T) {
	doc, err := Parse("````go\nfmt.Println(\"``` inside\")\n````")
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
	if got := block.Meta["fence"]; got != "4" {
		t.Fatalf("fence = %q, want 4", got)
	}
	if len(block.Lines) != 1 {
		t.Fatalf("lines len = %d, want 1", len(block.Lines))
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

func TestParseBuildsHeadingBlocks(t *testing.T) {
	doc, err := Parse("## Quick take\nNormal line")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("blocks len = %d, want 2", len(doc.Blocks))
	}
	block := doc.Blocks[0]
	if block.Kind != HeadingBlock {
		t.Fatalf("kind = %q, want heading", block.Kind)
	}
	if block.HeadingLevel != 2 {
		t.Fatalf("heading level = %d, want 2", block.HeadingLevel)
	}
	if got := renderTextBlock(block); got != "## Quick take\n" {
		t.Fatalf("rendered block = %q", got)
	}
}

func TestParseBuildsListBlock(t *testing.T) {
	doc, err := Parse("- item 1\n  - item 2\n1. item 3")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(doc.Blocks))
	}
	block := doc.Blocks[0]
	if block.Kind != ListBlock {
		t.Fatalf("kind = %q, want list", block.Kind)
	}
	if len(block.Items) != 3 {
		t.Fatalf("items len = %d, want 3", len(block.Items))
	}
	if block.Items[1].ListIndent != 2 {
		t.Fatalf("item[1] indent = %d, want 2", block.Items[1].ListIndent)
	}
	if !block.Items[2].Ordered {
		t.Fatalf("item[2] expected ordered")
	}
}
