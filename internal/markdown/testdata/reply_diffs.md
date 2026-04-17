Here are the heading-related diffs.

### `internal/markdown/ast.go`

```diff
 type LineNode struct {
- StartPos Position    `json:"start_pos"`
- EndPos   Position    `json:"end_pos"`
- Spans    []*SpanNode `json:"spans,omitempty"`
+ StartPos     Position    `json:"start_pos"`
+ EndPos       Position    `json:"end_pos"`
+ HeadingLevel int         `json:"heading_level,omitempty"`
+ Spans        []*SpanNode `json:"spans,omitempty"`
 }
@@
 func (l *LineNode) Size() int {
  if l == nil {
   return 0
  }
  total := 0
+ if l.HeadingLevel > 0 {
+  total += l.HeadingLevel + 1
+ }
  for _, span := range l.Spans {
   total += span.Size()
  }
@@
- out := &LineNode{StartPos: line.StartPos, EndPos: line.EndPos}
+ out := &LineNode{StartPos: line.StartPos, EndPos: line.EndPos, HeadingLevel: line.HeadingLevel}
```

### `internal/markdown/parser.go`

```diff
+import "strings"
@@
- return &LineNode{StartPos: start, EndPos: end, Spans: parseInlineTokens(tokens)}
+ headingLevel, tokens := detectHeadingTokens(tokens)
+ return &LineNode{StartPos: start, EndPos: end, HeadingLevel: headingLevel, Spans: parseInlineTokens(tokens)}
+}
+
+func detectHeadingTokens(tokens []Token) (int, []Token) {
+ if len(tokens) == 0 || tokens[0].Kind != TokenText {
+  return 0, tokens
+ }
+ text := tokens[0].Text
+ level := 0
+ rest := ""
+ switch {
+ case strings.HasPrefix(text, "### "):
+  level, rest = 3, text[4:]
+ case strings.HasPrefix(text, "## "):
+  level, rest = 2, text[3:]
+ case strings.HasPrefix(text, "# "):
+  level, rest = 1, text[2:]
+ default:
+  return 0, tokens
+ }
+ cloned := append([]Token(nil), tokens...)
+ cloned[0].Text = rest
+ return level, cloned
 }
```

### `internal/markdown/render_text.go`

```diff
 func renderTextLine(line *LineNode) string {
  if line == nil {
   return ""
  }
  var b strings.Builder
+ if line.HeadingLevel > 0 {
+  b.WriteString(strings.Repeat("#", line.HeadingLevel))
+  b.WriteString(" ")
+ }
  for _, span := range line.Spans {
   b.WriteString(renderTextSpan(span))
  }
```

### `internal/markdown/render_html.go`

```diff
 func renderHTMLLine(line *LineNode) string {
@@
- return b.String()
+ text := b.String()
+ if line.HeadingLevel > 0 {
+  return "<b>" + text + "</b>"
+ }
+ return text
 }
```

### `internal/markdown/render_markdown.go`

```diff
 func renderMarkdownLine(line *LineNode) string {
@@
- return b.String()
+ text := b.String()
+ if line.HeadingLevel > 0 {
+  return "*" + text + "*"
+ }
+ return text
 }
```

### `internal/markdown/parser_test.go`

```diff
+func TestParseDetectsHeadingLines(t *testing.T) {
+ doc, err := Parse("## Quick take\nNormal line")
+ if err != nil {
+  t.Fatalf("Parse: %v", err)
+ }
+ line := doc.Blocks[0].Lines[0]
+ if line.HeadingLevel != 2 {
+  t.Fatalf("heading level = %d, want 2", line.HeadingLevel)
+ }
+ if got := renderTextLine(line); got != "## Quick take" {
+  t.Fatalf("rendered line = %q", got)
+ }
+}
```

### `internal/markdown/render_test.go`

```diff
+func TestRenderHeadings(t *testing.T) {
+ doc, err := Parse("## Quick take")
+ if err != nil {
+  t.Fatalf("Parse: %v", err)
+ }
+ plain, err := doc.Render(RenderOptions{Format: RenderPlain})
+ if err != nil {
+  t.Fatalf("Render plain: %v", err)
+ }
+ if plain != "## Quick take" {
+  t.Fatalf("plain = %q", plain)
+ }
+ html, err := doc.Render(RenderOptions{Format: RenderHTML})
+ if err != nil {
+  t.Fatalf("Render html: %v", err)
+ }
+ if html != "<b>Quick take</b>" {
+  t.Fatalf("html = %q", html)
+ }
+ md, err := doc.Render(RenderOptions{Format: RenderMarkdownV2})
+ if err != nil {
+  t.Fatalf("Render markdown: %v", err)
+ }
+ if md != "*Quick take*" {
+  t.Fatalf("markdown = %q", md)
+ }
+}
```
