package markdownv4

import "testing"

func TestParseBuildsTokenDrivenDocumentTree(t *testing.T) {
	doc, err := Parse("Hello **bold** and *italic* and `code`.")
	if err != nil {
		t.Fatalf("Parse: %v", err)
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
		t.Fatalf("code info = %q, want go", got)
	}
	list := doc.Root.Children[1]
	if list.Kind != NodeList || len(list.Children) != 2 {
		t.Fatalf("list = %#v, want list with 2 items", list)
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

func TestValidateRejectsTopLevelListItem(t *testing.T) {
	err := Validate(&Node{Kind: NodeDocument, Children: []*Node{{Kind: NodeListItem}}})
	if err == nil {
		t.Fatalf("expected validation error")
	}
}

func TestValidateWalkAndClone(t *testing.T) {
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
