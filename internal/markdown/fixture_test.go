package markdown

import (
	"os"
	"path/filepath"
	"testing"
)

func loadFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("testdata", name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", path, err)
	}
	return string(data)
}

func TestFixturesChunkToPlainSizeLimit(t *testing.T) {
	fixtures := []string{
		"reply_simple.md",
		"reply_code_heavy.md",
		"reply_long_explanation.md",
	}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			doc, err := Parse(loadFixture(t, name))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			chunks := doc.Chunked(48)
			if len(chunks) == 0 {
				t.Fatalf("Chunked returned no chunks")
			}
			for i, chunk := range chunks {
				if chunk.Size() > 48 {
					t.Fatalf("chunk %d size = %d, want <= 48", i, chunk.Size())
				}
				text, err := chunk.Render(RenderOptions{Format: RenderPlain})
				if err != nil {
					t.Fatalf("Render plain: %v", err)
				}
				if text == "" {
					t.Fatalf("chunk %d rendered empty text", i)
				}
			}
		})
	}
}

func TestFixturesRenderAcrossFormats(t *testing.T) {
	fixtures := []string{
		"reply_simple.md",
		"reply_code_heavy.md",
		"reply_long_explanation.md",
	}
	formats := []RenderFormat{RenderPlain, RenderHTML, RenderMarkdownV2}
	for _, name := range fixtures {
		t.Run(name, func(t *testing.T) {
			doc, err := Parse(loadFixture(t, name))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			for _, chunk := range doc.Chunked(64) {
				for _, format := range formats {
					text, err := chunk.Render(RenderOptions{Format: format})
					if err != nil {
						t.Fatalf("Render(%s): %v", format, err)
					}
					if text == "" {
						t.Fatalf("Render(%s) returned empty text", format)
					}
				}
			}
		})
	}
}
