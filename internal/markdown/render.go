package markdown

import "fmt"

type RenderFormat string

const (
	RenderPlain      RenderFormat = "plain"
	RenderHTML       RenderFormat = "html"
	RenderMarkdownV2 RenderFormat = "markdown_v2"
)

type RenderOptions struct {
	Format    RenderFormat
	ChunkSize int
}

// Render renders one semantic chunk document in the requested format.
func (d *Document) Render(opts RenderOptions) (string, error) {
	if d == nil {
		return "", fmt.Errorf("document is nil")
	}
	format := opts.Format
	if format == "" {
		format = RenderPlain
	}
	switch format {
	case RenderPlain:
		return renderTextDocument(d), nil
	case RenderHTML:
		return renderHTMLDocument(d), nil
	case RenderMarkdownV2:
		return renderMarkdownDocument(d), nil
	default:
		return "", fmt.Errorf("unsupported render format %q", format)
	}
}
