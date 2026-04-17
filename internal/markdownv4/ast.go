package markdownv4

import "fmt"

type Position struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

type Span struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type NodeKind string

const (
	NodeDocument   NodeKind = "document"
	NodeParagraph  NodeKind = "paragraph"
	NodeCodeBlock  NodeKind = "code_block"
	NodeList       NodeKind = "list"
	NodeListItem   NodeKind = "list_item"
	NodeText       NodeKind = "text"
	NodeStrong     NodeKind = "strong"
	NodeEmphasis   NodeKind = "emphasis"
	NodeInlineCode NodeKind = "inline_code"
)

type Node struct {
	Kind     NodeKind          `json:"kind"`
	Text     string            `json:"text,omitempty"`
	Children []*Node           `json:"children,omitempty"`
	Span     Span              `json:"span"`
	Meta     map[string]string `json:"meta,omitempty"`
}

type Document struct {
	Root *Node `json:"root"`
}

func (d *Document) Validate() error {
	if d == nil {
		return fmt.Errorf("document is nil")
	}
	return Validate(d.Root)
}

func Validate(root *Node) error {
	if root == nil {
		return fmt.Errorf("root node is nil")
	}
	return validateNode(root, NodeDocument)
}

func validateNode(node *Node, rootKind NodeKind) error {
	if node == nil {
		return fmt.Errorf("node is nil")
	}
	switch node.Kind {
	case NodeDocument:
		if rootKind != NodeDocument {
			return fmt.Errorf("document node must be root")
		}
		if node.Text != "" {
			return fmt.Errorf("document node cannot carry text")
		}
		for _, child := range node.Children {
			if !isBlockKind(child.Kind) {
				return fmt.Errorf("document child kind %q is not a block", child.Kind)
			}
			if err := validateNode(child, ""); err != nil {
				return err
			}
		}
	case NodeParagraph:
		for _, child := range node.Children {
			if !isInlineKind(child.Kind) {
				return fmt.Errorf("paragraph child kind %q is not inline", child.Kind)
			}
			if err := validateNode(child, ""); err != nil {
				return err
			}
		}
	case NodeCodeBlock:
		if len(node.Children) != 0 {
			return fmt.Errorf("code_block cannot have children")
		}
	case NodeList:
		for _, child := range node.Children {
			if child.Kind != NodeListItem {
				return fmt.Errorf("list child kind %q must be list_item", child.Kind)
			}
			if err := validateNode(child, ""); err != nil {
				return err
			}
		}
	case NodeListItem:
		for _, child := range node.Children {
			if !isBlockKind(child.Kind) {
				return fmt.Errorf("list_item child kind %q is not a block", child.Kind)
			}
			if err := validateNode(child, ""); err != nil {
				return err
			}
		}
	case NodeText, NodeInlineCode:
		if len(node.Children) != 0 {
			return fmt.Errorf("%s node cannot have children", node.Kind)
		}
	case NodeStrong, NodeEmphasis:
		for _, child := range node.Children {
			if !isInlineKind(child.Kind) {
				return fmt.Errorf("%s child kind %q is not inline", node.Kind, child.Kind)
			}
			if err := validateNode(child, ""); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unsupported node kind %q", node.Kind)
	}
	return nil
}

func Walk(root *Node, fn func(*Node) bool) {
	if root == nil || fn == nil {
		return
	}
	if !fn(root) {
		return
	}
	for _, child := range root.Children {
		Walk(child, fn)
	}
}

func Clone(root *Node) *Node {
	if root == nil {
		return nil
	}
	out := &Node{Kind: root.Kind, Text: root.Text, Span: root.Span}
	if len(root.Meta) > 0 {
		out.Meta = make(map[string]string, len(root.Meta))
		for k, v := range root.Meta {
			out.Meta[k] = v
		}
	}
	if len(root.Children) > 0 {
		out.Children = make([]*Node, 0, len(root.Children))
		for _, child := range root.Children {
			out.Children = append(out.Children, Clone(child))
		}
	}
	return out
}

func isBlockKind(kind NodeKind) bool {
	switch kind {
	case NodeParagraph, NodeCodeBlock, NodeList:
		return true
	default:
		return false
	}
}

func isInlineKind(kind NodeKind) bool {
	switch kind {
	case NodeText, NodeStrong, NodeEmphasis, NodeInlineCode:
		return true
	default:
		return false
	}
}
