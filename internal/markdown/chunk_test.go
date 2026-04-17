package markdown

import "testing"

func TestDocumentChunkSplitsAcrossParagraphs(t *testing.T) {
	doc, err := Parse("alpha beta gamma\n\nsecond paragraph here")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks, err := doc.Chunk(24)
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("chunks len = %d, want 2", len(chunks))
	}
	for _, chunk := range chunks {
		if err := chunk.Validate(); err != nil {
			t.Fatalf("Validate(chunk): %v", err)
		}
		if len(chunk.Root.Children) != 1 || chunk.Root.Children[0].Kind != NodeParagraph {
			t.Fatalf("chunk root children = %#v, want single paragraph", chunk.Root.Children)
		}
	}
}

func TestDocumentChunkSplitsLongParagraphIntoParagraphDocuments(t *testing.T) {
	doc, err := Parse("**alpha beta gamma delta epsilon zeta eta theta**")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks, err := doc.Chunk(12)
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks len = %d, want at least 2", len(chunks))
	}
	for _, chunk := range chunks {
		if err := chunk.Validate(); err != nil {
			t.Fatalf("Validate(chunk): %v", err)
		}
		if len(chunk.Root.Children) != 1 || chunk.Root.Children[0].Kind != NodeParagraph {
			t.Fatalf("chunk root children = %#v, want single paragraph", chunk.Root.Children)
		}
		if len(chunk.Root.Children[0].Children) != 1 || chunk.Root.Children[0].Children[0].Kind != NodeStrong {
			t.Fatalf("paragraph children = %#v, want single strong node", chunk.Root.Children[0].Children)
		}
	}
}

func TestDocumentChunkSplitsLongCodeBlockIntoCodeBlockDocuments(t *testing.T) {
	doc, err := Parse("```go\nalpha beta gamma delta\nsecond line here\n```")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks, err := doc.Chunk(10)
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks len = %d, want at least 2", len(chunks))
	}
	for _, chunk := range chunks {
		if err := chunk.Validate(); err != nil {
			t.Fatalf("Validate(chunk): %v", err)
		}
		if len(chunk.Root.Children) != 1 || chunk.Root.Children[0].Kind != NodeCodeBlock {
			t.Fatalf("chunk root children = %#v, want single code block", chunk.Root.Children)
		}
		if got := chunk.Root.Children[0].Meta["info"]; got != "go" {
			t.Fatalf("code info = %q, want go", got)
		}
	}
}

func TestDocumentChunkSplitsLongListItemIntoListDocuments(t *testing.T) {
	doc, err := Parse("- alpha beta gamma delta epsilon zeta eta theta")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks, err := doc.Chunk(12)
	if err != nil {
		t.Fatalf("Chunk: %v", err)
	}
	if len(chunks) < 2 {
		t.Fatalf("chunks len = %d, want at least 2", len(chunks))
	}
	for _, chunk := range chunks {
		if err := chunk.Validate(); err != nil {
			t.Fatalf("Validate(chunk): %v", err)
		}
		if len(chunk.Root.Children) != 1 || chunk.Root.Children[0].Kind != NodeList {
			t.Fatalf("chunk root children = %#v, want single list", chunk.Root.Children)
		}
		list := chunk.Root.Children[0]
		if len(list.Children) != 1 || list.Children[0].Kind != NodeListItem {
			t.Fatalf("list children = %#v, want single list item", list.Children)
		}
	}
}
