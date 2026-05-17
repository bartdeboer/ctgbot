package semantic

import (
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/go-clir"
)

func TestSearchOutputOptionsCapsDefaultAndFullOutput(t *testing.T) {
	opts, err := searchOutputOptions(ComponentConfig{Limit: 10}, strategySearchCommand{})
	if err != nil {
		t.Fatalf("searchOutputOptions(default) error = %v", err)
	}
	if opts.Limit != 10 || opts.ExcerptSize != DefaultExcerptSize || opts.Full {
		t.Fatalf("default opts = %#v", opts)
	}

	opts, err = searchOutputOptions(ComponentConfig{Limit: 10}, strategySearchCommand{Full: true})
	if err != nil {
		t.Fatalf("searchOutputOptions(full default) error = %v", err)
	}
	if opts.Limit != DefaultFullSearchResults || !opts.Full {
		t.Fatalf("full opts = %#v", opts)
	}

	if _, err := searchOutputOptions(ComponentConfig{Limit: 10}, strategySearchCommand{Limit: MaxSearchResults + 1}); err == nil {
		t.Fatalf("searchOutputOptions(limit too high) error = nil")
	}
	if _, err := searchOutputOptions(ComponentConfig{Limit: 10}, strategySearchCommand{Limit: -1}); err == nil {
		t.Fatalf("searchOutputOptions(negative limit) error = nil")
	}
	if _, err := searchOutputOptions(ComponentConfig{Limit: 10}, strategySearchCommand{Full: true, Limit: MaxFullSearchResults + 1}); err == nil {
		t.Fatalf("searchOutputOptions(full limit too high) error = nil")
	}
	if _, err := searchOutputOptions(ComponentConfig{Limit: 10}, strategySearchCommand{ExcerptSize: MaxExcerptSize + 1}); err == nil {
		t.Fatalf("searchOutputOptions(excerpt too high) error = nil")
	}
}

func TestBuildStrategySearchCommandParsesOutputSafetyFlags(t *testing.T) {
	built, err := buildStrategySearchCommand(&clir.Request{
		Params: map[string]string{"strategy": "qwen", "query": "mailbox storage"},
		Extra:  []string{"--limit", "3", "--excerpt-size", "500", "--full", "--all"},
	})
	if err != nil {
		t.Fatalf("buildStrategySearchCommand() error = %v", err)
	}
	cmd := built.(strategySearchCommand)
	if cmd.Strategy != "qwen" || cmd.Query != "mailbox storage" || cmd.Limit != 3 || cmd.ExcerptSize != 500 || !cmd.Full || !cmd.Scope.All {
		t.Fatalf("cmd = %#v", cmd)
	}
}

func TestRenderSearchResponseUsesBoundedExcerpt(t *testing.T) {
	msgID := modeluuid.New()
	threadID := modeluuid.New()
	text := "one two three four five six seven"
	rendered := renderSearchResponse("qwen: words", time.Second, component.SearchResponse{Results: []component.SearchResult{{
		MessageID: msgID,
		ThreadID:  threadID,
		Text:      text,
		Score:     0.9,
	}}}, searchOutput{Limit: 1, ExcerptSize: 13})
	if !strings.Contains(rendered, "excerpt: one two three...") {
		t.Fatalf("rendered = %s", rendered)
	}
	if strings.Contains(rendered, "four five six") {
		t.Fatalf("rendered contains unbounded text = %s", rendered)
	}
}

func TestRenderSearchResponseFullFencesText(t *testing.T) {
	msgID := modeluuid.New()
	threadID := modeluuid.New()
	text := "line one\n```text\nnested\n```"
	rendered := renderSearchResponse("qwen: fenced", time.Second, component.SearchResponse{Results: []component.SearchResult{{
		MessageID: msgID,
		ThreadID:  threadID,
		Text:      text,
		Score:     0.9,
	}}}, searchOutput{Limit: 1, ExcerptSize: DefaultExcerptSize, Full: true})
	if !strings.Contains(rendered, "text:\n````text\n") {
		t.Fatalf("rendered missing safe fence = %s", rendered)
	}
	if strings.Contains(rendered, "excerpt:") {
		t.Fatalf("rendered full output should not include excerpt = %s", rendered)
	}
}
