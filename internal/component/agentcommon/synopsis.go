package agentcommon

import (
	"sort"
	"strings"
)

// HostbridgeSynopsis renders command patterns as a compact command-tree synopsis.
// Inputs may be full commands ("hostbridge codex status") or command tails
// ("codex status"). Empty inputs render as "hostbridge [\n  <none>\n]".
func HostbridgeSynopsis(patterns []string) string {
	return CommandSynopsis("hostbridge", stripCommandPrefix(patterns, "hostbridge"))
}

// CommandSynopsis renders route-like command patterns as a readable trie.
// It is intended for developer instructions where agents benefit from a compact
// grammar rather than a long flat list.
func CommandSynopsis(root string, patterns []string) string {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "commands"
	}
	trie := newSynopsisNode("")
	for _, pattern := range patterns {
		for _, expanded := range expandSynopsisPattern(pattern) {
			trie.insert(strings.Fields(expanded))
		}
	}
	if len(trie.children) == 0 {
		return root + " [\n  <none>\n]"
	}
	lines := []string{root + " ["}
	for _, child := range trie.orderedChildren() {
		lines = append(lines, renderSynopsisChild(child, "  ")...)
	}
	lines = append(lines, "]")
	return strings.Join(lines, "\n")
}

type synopsisNode struct {
	token    string
	terminal bool
	children map[string]*synopsisNode
	order    []string
}

func newSynopsisNode(token string) *synopsisNode {
	return &synopsisNode{token: token, children: map[string]*synopsisNode{}}
}

func (n *synopsisNode) insert(tokens []string) {
	if len(tokens) == 0 {
		n.terminal = true
		return
	}
	token := tokens[0]
	child := n.children[token]
	if child == nil {
		child = newSynopsisNode(token)
		n.children[token] = child
		n.order = append(n.order, token)
	}
	child.insert(tokens[1:])
}

func (n *synopsisNode) orderedChildren() []*synopsisNode {
	out := make([]*synopsisNode, 0, len(n.children))
	for _, token := range n.order {
		if child := n.children[token]; child != nil {
			out = append(out, child)
		}
	}
	return out
}

func renderSynopsisChild(n *synopsisNode, indent string) []string {
	if n == nil {
		return nil
	}
	if len(n.children) == 0 {
		return []string{indent + n.token}
	}
	inline := renderSynopsisInline(n)
	if len(inline) <= 88 && !strings.Contains(inline, "\n") {
		return []string{indent + inline}
	}
	lines := []string{indent + n.token + " ["}
	if n.terminal {
		lines = append(lines, indent+"  .")
	}
	for _, child := range n.orderedChildren() {
		lines = append(lines, renderSynopsisChild(child, indent+"  ")...)
	}
	lines = append(lines, indent+"]")
	return lines
}

func renderSynopsisInline(n *synopsisNode) string {
	if n == nil {
		return ""
	}
	if len(n.children) == 0 {
		return n.token
	}
	children := n.orderedChildren()
	if len(children) == 1 && !n.terminal {
		return strings.TrimSpace(n.token + " " + renderSynopsisInline(children[0]))
	}
	parts := make([]string, 0, len(children)+1)
	if n.terminal {
		parts = append(parts, ".")
	}
	for _, child := range children {
		parts = append(parts, renderSynopsisInline(child))
	}
	return n.token + " [ " + strings.Join(parts, " | ") + " ]"
}

func stripCommandPrefix(patterns []string, prefix string) []string {
	prefix = strings.TrimSpace(prefix)
	out := make([]string, 0, len(patterns))
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if prefix != "" && pattern == prefix {
			pattern = ""
		} else if prefix != "" && strings.HasPrefix(pattern, prefix+" ") {
			pattern = strings.TrimSpace(strings.TrimPrefix(pattern, prefix+" "))
		}
		out = append(out, pattern)
	}
	return out
}

func expandSynopsisPattern(pattern string) []string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil
	}
	open := strings.Index(pattern, "[")
	close := strings.LastIndex(pattern, "]")
	if open < 0 || close < open {
		return []string{pattern}
	}
	prefix := strings.TrimSpace(pattern[:open])
	suffix := strings.TrimSpace(pattern[close+1:])
	body := strings.TrimSpace(pattern[open+1 : close])
	choices := strings.Split(body, "|")
	out := make([]string, 0, len(choices))
	for _, choice := range choices {
		parts := []string{prefix, strings.TrimSpace(choice), suffix}
		out = append(out, strings.TrimSpace(strings.Join(nonEmpty(parts), " ")))
	}
	sort.Strings(out)
	return out
}

func nonEmpty(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	return out
}
