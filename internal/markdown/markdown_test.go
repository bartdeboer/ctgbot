package markdown

import (
	"strings"
	"testing"
)

func TestRenderChunkedPlainPreservesParagraphs(t *testing.T) {
	doc := New("Hello **world**.\n\nSecond paragraph.")
	chunks, err := doc.RenderChunked(RenderOptions{Format: RenderPlain, ChunkSize: 3500})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("chunks len = %d, want 1", len(chunks))
	}
	if got := chunks[0].Text; got != "Hello world.\n\nSecond paragraph." {
		t.Fatalf("chunk text = %q", got)
	}
}

func TestRenderChunkedHTMLRendersInlineFormatting(t *testing.T) {
	doc := New("Use **bold** and *italic* and `code`.")
	chunks, err := doc.RenderChunked(RenderOptions{Format: RenderHTML, ChunkSize: 3500})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("chunks len = %d, want 1", len(chunks))
	}
	want := "Use <b>bold</b> and <i>italic</i> and <code>code</code>."
	if chunks[0].Text != want {
		t.Fatalf("chunk text = %q, want %q", chunks[0].Text, want)
	}
}

func TestRenderChunkedHTMLSplitsLongBoldParagraphIntoBalancedChunks(t *testing.T) {
	doc := New("**alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu**")
	chunks, err := doc.RenderChunked(RenderOptions{Format: RenderHTML, ChunkSize: 30})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks len = %d, want at least 2", len(chunks))
	}
	for _, chunk := range chunks {
		if strings.Count(chunk.Text, "<b>") != strings.Count(chunk.Text, "</b>") {
			t.Fatalf("unbalanced bold tags in %q", chunk.Text)
		}
		if strings.Contains(chunk.Text, "**") {
			t.Fatalf("raw markdown leaked into %q", chunk.Text)
		}
	}
}

func TestRenderChunkedHTMLSplitsCodeBlocksSafely(t *testing.T) {
	doc := New("```\nline one\nline two\nline three\nline four\n```")
	chunks, err := doc.RenderChunked(RenderOptions{Format: RenderHTML, ChunkSize: 24})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks len = %d, want at least 2", len(chunks))
	}
	for _, chunk := range chunks {
		if !strings.HasPrefix(chunk.Text, "<pre><code>") || !strings.HasSuffix(chunk.Text, "</code></pre>") {
			t.Fatalf("invalid code block chunk %q", chunk.Text)
		}
	}
}
