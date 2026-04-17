package markdown

import "testing"

func TestDocumentSliceTextRange(t *testing.T) {
	doc, err := Parse("hello world")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	sliced := doc.Slice(Position{Offset: 6}, Position{Offset: 11})
	if err := sliced.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	chunks, err := sliced.RenderChunked(RenderOptions{Format: RenderPlain, ChunkSize: 100})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	if len(chunks) != 1 || chunks[0].Text != "world" {
		t.Fatalf("chunks = %#v, want world", chunks)
	}
}

func TestDocumentSlicePreservesStrongStructure(t *testing.T) {
	src := "a **bold** z"
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	sliced := doc.Slice(Position{Offset: 3}, Position{Offset: 9})
	if err := sliced.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	para := sliced.Root.Children[0]
	if para.Kind != NodeParagraph {
		t.Fatalf("kind = %q, want paragraph", para.Kind)
	}
	if len(para.Children) != 1 || para.Children[0].Kind != NodeStrong {
		t.Fatalf("children = %#v, want single strong node", para.Children)
	}
	chunks, err := sliced.RenderChunked(RenderOptions{Format: RenderPlain, ChunkSize: 100})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	if chunks[0].Text != "bold" {
		t.Fatalf("text = %q, want bold", chunks[0].Text)
	}
}

func TestDocumentSlicePreservesCodeBlockInfo(t *testing.T) {
	src := "```go\nline one\nline two\n```"
	doc, err := Parse(src)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	sliced := doc.Slice(Position{Offset: 6}, Position{Offset: 14})
	if err := sliced.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if len(sliced.Root.Children) != 1 || sliced.Root.Children[0].Kind != NodeCodeBlock {
		t.Fatalf("children = %#v, want single code block", sliced.Root.Children)
	}
	if got := sliced.Root.Children[0].Meta["info"]; got != "go" {
		t.Fatalf("info = %q, want go", got)
	}
	if got := sliced.Root.Children[0].Text; got != "line one" {
		t.Fatalf("text = %q, want line one", got)
	}
}
