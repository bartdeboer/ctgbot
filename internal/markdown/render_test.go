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

func TestRenderMarkdownIndentedFence(t *testing.T) {
	doc, err := Parse("  ```go\n  fmt.Println(\"hi\")\n  ```")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := doc.Render(RenderOptions{Format: RenderMarkdownV2})
	if err != nil {
		t.Fatalf("Render markdown: %v", err)
	}
	want := "```go\nfmt.Println(\"hi\")\n```"
	if got != want {
		t.Fatalf("markdown output = %q, want %q", got, want)
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

func TestRenderPlainListBlock(t *testing.T) {
	doc, err := Parse("- item 1\n  - item 2\n1. item 3")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := doc.Render(RenderOptions{Format: RenderPlain})
	if err != nil {
		t.Fatalf("Render plain: %v", err)
	}
	want := "- item 1\n  - item 2\n1. item 3"
	if got != want {
		t.Fatalf("plain = %q, want %q", got, want)
	}
}

func TestRenderMarkdownListBlock(t *testing.T) {
	doc, err := Parse(`uploads saved:
- /workspace/inbox/photo-1408.jpg
  - child-item
1. item 3`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := doc.Render(RenderOptions{Format: RenderMarkdownV2})
	if err != nil {
		t.Fatalf("Render markdown: %v", err)
	}
	want := `uploads saved:

\- /workspace/inbox/photo\-1408\.jpg
  \- child\-item
1\. item 3`
	if got != want {
		t.Fatalf("markdown = %q, want %q", got, want)
	}
}

func TestRenderMarkdownListContinuationDoesNotCreateParagraphGap(t *testing.T) {
	doc, err := Parse(`- **Workspace separation** — chats can be attached to named workspaces, so
  the same bot can operate on different projects without mixing directories.`)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	got, err := doc.Render(RenderOptions{Format: RenderMarkdownV2})
	if err != nil {
		t.Fatalf("Render markdown: %v", err)
	}
	want := `\- *Workspace separation* — chats can be attached to named workspaces, so
the same bot can operate on different projects without mixing directories\.`
	if got != want {
		t.Fatalf("markdown = %q, want %q", got, want)
	}
}
