package markdownv3

import "testing"

func TestParseBuildsDocumentNodeTree(t *testing.T) {
	doc, err := Parse("Hello **bold** and *italic* and `code`.")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Root == nil || doc.Root.Kind != NodeDocument {
		t.Fatalf("root = %#v, want document node", doc.Root)
	}
	if len(doc.Root.Children) != 1 {
		t.Fatalf("root children len = %d, want 1", len(doc.Root.Children))
	}
	para := doc.Root.Children[0]
	if para.Kind != NodeParagraph {
		t.Fatalf("child[0] kind = %q, want paragraph", para.Kind)
	}
	if len(para.Children) != 7 {
		t.Fatalf("paragraph children len = %d, want 7", len(para.Children))
	}
	if para.Children[1].Kind != NodeStrong {
		t.Fatalf("child[1] kind = %q, want strong", para.Children[1].Kind)
	}
	if para.Children[3].Kind != NodeEmphasis {
		t.Fatalf("child[3] kind = %q, want emphasis", para.Children[3].Kind)
	}
	if para.Children[5].Kind != NodeInlineCode {
		t.Fatalf("child[5] kind = %q, want inline_code", para.Children[5].Kind)
	}
}

func TestParseCodeBlockAndList(t *testing.T) {
	doc, err := Parse("```go\nfmt.Println(\"hi\")\n```\n\n- first\n- second")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(doc.Root.Children) != 2 {
		t.Fatalf("root children len = %d, want 2", len(doc.Root.Children))
	}
	if doc.Root.Children[0].Kind != NodeCodeBlock {
		t.Fatalf("child[0] kind = %q, want code_block", doc.Root.Children[0].Kind)
	}
	if got := doc.Root.Children[0].Meta["info"]; got != "go" {
		t.Fatalf("code info = %q, want %q", got, "go")
	}
	list := doc.Root.Children[1]
	if list.Kind != NodeList {
		t.Fatalf("child[1] kind = %q, want list", list.Kind)
	}
	if len(list.Children) != 2 {
		t.Fatalf("list items len = %d, want 2", len(list.Children))
	}
}

func TestParseNestedInlineNodes(t *testing.T) {
	doc, err := Parse("**bold *and italic***")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	strong := doc.Root.Children[0].Children[0]
	if strong.Kind != NodeStrong {
		t.Fatalf("node kind = %q, want strong", strong.Kind)
	}
	if len(strong.Children) != 2 {
		t.Fatalf("strong children len = %d, want 2", len(strong.Children))
	}
	if strong.Children[1].Kind != NodeEmphasis {
		t.Fatalf("strong child[1] kind = %q, want emphasis", strong.Children[1].Kind)
	}
}

func TestParseMalformedFormattingFallsBackToText(t *testing.T) {
	doc, err := Parse("hello **world")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	text := doc.Root.Children[0].Children[0]
	if text.Kind != NodeText || text.Text != "hello **world" {
		t.Fatalf("text node = %#v, want raw text fallback", text)
	}
}

func TestValidateRejectsInvalidTree(t *testing.T) {
	err := Validate(&Node{Kind: NodeParagraph, Children: []*Node{{Kind: NodeParagraph}}})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestWalkAndClone(t *testing.T) {
	doc, err := Parse("- first\n- second")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	count := 0
	Walk(doc.Root, func(node *Node) bool {
		count++
		return true
	})
	if count == 0 {
		t.Fatalf("walk count = 0")
	}
	clone := Clone(doc.Root)
	if clone == doc.Root {
		t.Fatalf("clone reused root pointer")
	}
	if err := Validate(clone); err != nil {
		t.Fatalf("Validate(clone): %v", err)
	}
}
