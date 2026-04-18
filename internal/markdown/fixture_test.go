package markdown

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
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

func TestReplyDiffsFirstChunkRenderedUnderTelegramLimit(t *testing.T) {
	doc, err := Parse(loadFixture(t, "reply_diffs.md"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	chunks := doc.Chunked(3500)
	if len(chunks) == 0 {
		t.Fatalf("Chunked returned no chunks")
	}
	first := chunks[0]
	got, err := first.Render(RenderOptions{Format: RenderMarkdownV2})
	if err != nil {
		t.Fatalf("Render markdown: %v", err)
	}
	if n := utf8.RuneCountInString(got); n > 4096 {
		t.Fatalf("first chunk markdown length = %d, want <= 4096", n)
	}
}

func TestAgentFixtureCurrentRenderOutputs(t *testing.T) {
	doc, err := Parse(loadFixture(t, "agent_input.md"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	tests := []struct {
		name   string
		format RenderFormat
		want   string
	}{
		{name: "plain", format: RenderPlain, want: normalizeFixtureOutput(loadFixture(t, "agent_text.txt"))},
		{name: "html", format: RenderHTML, want: normalizeFixtureOutput(loadFixture(t, "agent_html.html"))},
		{name: "markdown", format: RenderMarkdownV2, want: normalizeFixtureOutput(loadFixture(t, "agent_md.md"))},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := doc.Render(RenderOptions{Format: tt.format})
			if err != nil {
				t.Fatalf("Render(%s): %v", tt.format, err)
			}
			if got != tt.want {
				t.Fatalf("Render(%s) mismatch\n--- got ---\n%s\n--- want ---\n%s", tt.format, got, tt.want)
			}
		})
	}
}

func normalizeFixtureOutput(text string) string {
	return strings.TrimSuffix(text, "\n")
}
