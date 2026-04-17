package markdown

import "testing"

func renderChunkDocs(t *testing.T, doc *Document, format RenderFormat, limit int) []string {
	t.Helper()
	docs := doc.Chunked(limit)
	out := make([]string, 0, len(docs))
	for _, chunkDoc := range docs {
		text, err := chunkDoc.Render(RenderOptions{Format: format})
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		out = append(out, text)
	}
	return out
}

func TestRenderPlainParagraphs(t *testing.T) {
	doc, err := Parse("Hello **world**.\nNext line.\n\n```go\nfmt.Println(\"hi\")\n```")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks := renderChunkDocs(t, doc, RenderPlain, 200)
	want := "Hello **world**.\nNext line.\n\n```go\nfmt.Println(\"hi\")\n```"
	if len(chunks) != 1 || chunks[0] != want {
		t.Fatalf("chunks = %#v, want %q", chunks, want)
	}
}

func TestRenderHTMLFormatting(t *testing.T) {
	doc, err := Parse("Hello **world** and *friends* and `code`.")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks := renderChunkDocs(t, doc, RenderHTML, 200)
	want := "Hello <b>world</b> and <i>friends</i> and <code>code</code>."
	if len(chunks) != 1 || chunks[0] != want {
		t.Fatalf("chunks = %#v, want %q", chunks, want)
	}
}

func TestRenderMarkdownCodeBlockInfo(t *testing.T) {
	doc, err := Parse("```go\nfmt.Println(\"hi\")\n```")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks := renderChunkDocs(t, doc, RenderMarkdownV2, 200)
	want := "```go\nfmt.Println(\"hi\")\n```"
	if len(chunks) != 1 || chunks[0] != want {
		t.Fatalf("chunks = %#v, want %q", chunks, want)
	}
}

func TestRenderMarkdownCodeBlockEscapesOnlyCodeChars(t *testing.T) {
	doc, err := Parse("```diff\n+ value := call(x)\npath := `quoted`\\here\n```")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := doc.Render(RenderOptions{Format: RenderMarkdownV2})
	if err != nil {
		t.Fatalf("Render markdown: %v", err)
	}
	want := "```diff\n+ value := call(x)\npath := \\`quoted\\`\\\\here\n```"
	if got != want {
		t.Fatalf("markdown output = %q, want %q", got, want)
	}
}

func TestChunkedSplitsByLines(t *testing.T) {
	doc, err := Parse("alpha line\nbeta line\ngamma line")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks := renderChunkDocs(t, doc, RenderPlain, 18)
	if len(chunks) != 3 {
		t.Fatalf("chunks len = %d, want 3", len(chunks))
	}
}

func TestChunkedSplitsOversizedCodeLine(t *testing.T) {
	doc, err := Parse("```go\nalpha beta gamma delta epsilon\n```")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks := renderChunkDocs(t, doc, RenderMarkdownV2, 20)
	if len(chunks) < 2 {
		t.Fatalf("chunks len = %d, want at least 2", len(chunks))
	}
	for _, chunk := range chunks {
		if len(chunk) == 0 || chunk[:5] != "```go" {
			t.Fatalf("invalid code chunk %q", chunk)
		}
	}
}

func TestRenderHeadings(t *testing.T) {
	doc, err := Parse("## Quick take")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	plain, err := doc.Render(RenderOptions{Format: RenderPlain})
	if err != nil {
		t.Fatalf("Render plain: %v", err)
	}
	if plain != "## Quick take" {
		t.Fatalf("plain = %q", plain)
	}
	html, err := doc.Render(RenderOptions{Format: RenderHTML})
	if err != nil {
		t.Fatalf("Render html: %v", err)
	}
	if html != "<b>Quick take</b>" {
		t.Fatalf("html = %q", html)
	}
	md, err := doc.Render(RenderOptions{Format: RenderMarkdownV2})
	if err != nil {
		t.Fatalf("Render markdown: %v", err)
	}
	if md != "*Quick take*" {
		t.Fatalf("markdown = %q", md)
	}
}
