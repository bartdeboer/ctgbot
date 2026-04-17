package markdown

import "testing"

func TestRenderChunkedPlainParagraphs(t *testing.T) {
	doc, err := Parse("Hello **world**.\nNext line.\n\n```go\nfmt.Println(\"hi\")\n```")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks, err := doc.RenderChunked(RenderOptions{Format: RenderPlain, ChunkSize: 200})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	want := "Hello world.\nNext line.\n\nfmt.Println(\"hi\")"
	if len(chunks) != 1 || chunks[0].Text != want {
		t.Fatalf("chunks = %#v, want %q", chunks, want)
	}
}

func TestRenderChunkedHTMLFormatting(t *testing.T) {
	doc, err := Parse("Hello **world** and *friends* and `code`.")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks, err := doc.RenderChunked(RenderOptions{Format: RenderHTML, ChunkSize: 200})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	want := "Hello <b>world</b> and <i>friends</i> and <code>code</code>."
	if len(chunks) != 1 || chunks[0].Text != want {
		t.Fatalf("chunks = %#v, want %q", chunks, want)
	}
}

func TestRenderChunkedMarkdownCodeBlockInfo(t *testing.T) {
	doc, err := Parse("```go\nfmt.Println(\"hi\")\n```")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks, err := doc.RenderChunked(RenderOptions{Format: RenderMarkdownV2, ChunkSize: 200})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	want := "```go\nfmt.Println(\"hi\")\n```"
	if len(chunks) != 1 || chunks[0].Text != want {
		t.Fatalf("chunks = %#v, want %q", chunks, want)
	}
}

func TestRenderChunkedSplitsByLines(t *testing.T) {
	doc, err := Parse("alpha line\nbeta line\ngamma line")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks, err := doc.RenderChunked(RenderOptions{Format: RenderPlain, ChunkSize: 18})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	if len(chunks) != 3 {
		t.Fatalf("chunks len = %d, want 3", len(chunks))
	}
}

func TestRenderChunkedSplitsOversizedCodeLine(t *testing.T) {
	doc, err := Parse("```go\nalpha beta gamma delta epsilon\n```")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks, err := doc.RenderChunked(RenderOptions{Format: RenderMarkdownV2, ChunkSize: 20})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks len = %d, want at least 2", len(chunks))
	}
	for _, chunk := range chunks {
		if len(chunk.Text) == 0 || chunk.Text[:5] != "```go" {
			t.Fatalf("invalid code chunk %q", chunk.Text)
		}
	}
}
