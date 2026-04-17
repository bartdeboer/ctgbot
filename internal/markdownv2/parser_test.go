package markdownv2

import "testing"

func TestParseParagraphWithInlineFormatting(t *testing.T) {
	doc, err := Parse("Hello **bold** and *italic* and `code`.")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(doc.Blocks))
	}
	para, ok := doc.Blocks[0].(Paragraph)
	if !ok {
		t.Fatalf("first block = %T, want Paragraph", doc.Blocks[0])
	}
	if len(para.Inlines) != 7 {
		t.Fatalf("inlines len = %d, want 7", len(para.Inlines))
	}
	if _, ok := para.Inlines[1].(Strong); !ok {
		t.Fatalf("inline[1] = %T, want Strong", para.Inlines[1])
	}
	if _, ok := para.Inlines[3].(Emphasis); !ok {
		t.Fatalf("inline[3] = %T, want Emphasis", para.Inlines[3])
	}
	if _, ok := para.Inlines[5].(InlineCode); !ok {
		t.Fatalf("inline[5] = %T, want InlineCode", para.Inlines[5])
	}
	if tail, ok := para.Inlines[6].(Text); !ok || tail.Value != "." {
		t.Fatalf("inline[6] = %#v, want trailing text", para.Inlines[6])
	}
}

func TestParseFencedCodeBlock(t *testing.T) {
	doc, err := Parse("before\n\n```\nline one\nline two\n```\n\nafter")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Blocks) != 3 {
		t.Fatalf("blocks len = %d, want 3", len(doc.Blocks))
	}
	code, ok := doc.Blocks[1].(CodeBlock)
	if !ok {
		t.Fatalf("block[1] = %T, want CodeBlock", doc.Blocks[1])
	}
	if code.Text != "line one\nline two" {
		t.Fatalf("code text = %q", code.Text)
	}
}

func TestParseList(t *testing.T) {
	doc, err := Parse("- first\n- second\n- third")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("blocks len = %d, want 1", len(doc.Blocks))
	}
	list, ok := doc.Blocks[0].(List)
	if !ok {
		t.Fatalf("block[0] = %T, want List", doc.Blocks[0])
	}
	if len(list.Items) != 3 {
		t.Fatalf("list items len = %d, want 3", len(list.Items))
	}
}

func TestParseNestedStrongAndEmphasis(t *testing.T) {
	doc, err := Parse("**bold *and italic***")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	para := doc.Blocks[0].(Paragraph)
	strong, ok := para.Inlines[0].(Strong)
	if !ok {
		t.Fatalf("inline[0] = %T, want Strong", para.Inlines[0])
	}
	if len(strong.Children) != 2 {
		t.Fatalf("strong children len = %d, want 2", len(strong.Children))
	}
	if _, ok := strong.Children[1].(Emphasis); !ok {
		t.Fatalf("strong child[1] = %T, want Emphasis", strong.Children[1])
	}
}

func TestParseMalformedFormattingFallsBackToText(t *testing.T) {
	doc, err := Parse("hello **world")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	para := doc.Blocks[0].(Paragraph)
	if len(para.Inlines) != 1 {
		t.Fatalf("inlines len = %d, want 1", len(para.Inlines))
	}
	text, ok := para.Inlines[0].(Text)
	if !ok {
		t.Fatalf("inline[0] = %T, want Text", para.Inlines[0])
	}
	if text.Value != "hello **world" {
		t.Fatalf("text value = %q", text.Value)
	}
}
