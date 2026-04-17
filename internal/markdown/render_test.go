package markdown

import (
	"strings"
	"testing"
)

func TestRenderChunkedPlainRendersParagraphsAndLists(t *testing.T) {
	doc, err := Parse("Hello **world**.\n\n- first\n- second")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks, err := doc.RenderChunked(RenderOptions{Format: RenderPlain, ChunkSize: 3500})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	if len(chunks) != 1 {
		t.Fatalf("chunks len = %d, want 1", len(chunks))
	}
	want := "Hello world.\n\n• first\n\n• second"
	if chunks[0].Text != want {
		t.Fatalf("chunk text = %q, want %q", chunks[0].Text, want)
	}
}

func TestRenderChunkedHTMLRendersInlineFormatting(t *testing.T) {
	doc, err := Parse("Use **bold** and *italic* and `code`.")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks, err := doc.RenderChunked(RenderOptions{Format: RenderHTML, ChunkSize: 3500})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	want := "Use <b>bold</b> and <i>italic</i> and <code>code</code>."
	if len(chunks) != 1 || chunks[0].Text != want {
		t.Fatalf("chunks = %#v, want %q", chunks, want)
	}
}

func TestRenderChunkedHTMLEscapesSpecialCharacters(t *testing.T) {
	doc, err := Parse("5 < 7 & **A > B** and `x < y && z`")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks, err := doc.RenderChunked(RenderOptions{Format: RenderHTML, ChunkSize: 3500})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	got := chunks[0].Text
	if !strings.Contains(got, "5 &lt; 7 &amp; ") {
		t.Fatalf("expected escaped plain text, got %q", got)
	}
	if !strings.Contains(got, "<b>A &gt; B</b>") {
		t.Fatalf("expected escaped bold text, got %q", got)
	}
	if !strings.Contains(got, "<code>x &lt; y &amp;&amp; z</code>") {
		t.Fatalf("expected escaped code text, got %q", got)
	}
}

func TestRenderChunkedHTMLSplitsLongBoldParagraphIntoBalancedChunks(t *testing.T) {
	doc, err := Parse("**alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu**")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
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

func TestRenderChunkedHTMLSplitsLongListItemsWithBulletPrefix(t *testing.T) {
	doc, err := Parse("- **alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu**")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks, err := doc.RenderChunked(RenderOptions{Format: RenderHTML, ChunkSize: 32})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks len = %d, want at least 2", len(chunks))
	}
	for _, chunk := range chunks {
		if !strings.HasPrefix(chunk.Text, "• ") {
			t.Fatalf("chunk missing bullet prefix: %q", chunk.Text)
		}
	}
}

func TestRenderChunkedHTMLSplitsInlineCodeSafely(t *testing.T) {
	doc, err := Parse("`alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu`")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks, err := doc.RenderChunked(RenderOptions{Format: RenderHTML, ChunkSize: 28})
	if err != nil {
		t.Fatalf("RenderChunked: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks len = %d, want at least 2", len(chunks))
	}
	for _, chunk := range chunks {
		if strings.Count(chunk.Text, "<code>") != strings.Count(chunk.Text, "</code>") {
			t.Fatalf("unbalanced code tags in %q", chunk.Text)
		}
	}
}

func TestRenderChunkedHTMLSplitsCodeBlocksSafely(t *testing.T) {
	doc, err := Parse("```\nline one\nline two\nline three\nline four\n```")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
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
