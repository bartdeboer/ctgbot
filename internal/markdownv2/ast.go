package markdownv2

type Position struct {
	Offset int `json:"offset"`
	Line   int `json:"line"`
	Column int `json:"column"`
}

type Span struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Document struct {
	Blocks []Block `json:"blocks"`
}

type Block interface {
	isBlock()
}

type Inline interface {
	isInline()
}

type Paragraph struct {
	Inlines []Inline `json:"inlines"`
	Span    Span     `json:"span"`
}

func (Paragraph) isBlock() {}

type CodeBlock struct {
	Text string `json:"text"`
	Span Span   `json:"span"`
}

func (CodeBlock) isBlock() {}

type List struct {
	Items []ListItem `json:"items"`
	Span  Span       `json:"span"`
}

func (List) isBlock() {}

type ListItem struct {
	Blocks []Block `json:"blocks"`
	Span   Span    `json:"span"`
}

type Text struct {
	Value string `json:"value"`
}

func (Text) isInline() {}

type Strong struct {
	Children []Inline `json:"children"`
}

func (Strong) isInline() {}

type Emphasis struct {
	Children []Inline `json:"children"`
}

func (Emphasis) isInline() {}

type InlineCode struct {
	Text string `json:"text"`
}

func (InlineCode) isInline() {}
