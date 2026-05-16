package semantic

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func TestParseScoreResponseAcceptsJSONFence(t *testing.T) {
	parsed, err := parseScoreResponse("```json\n{\"scores\":[{\"id\":\"m1\",\"score\":0.7,\"reason\":\"related\"}]}\n```")
	if err != nil {
		t.Fatalf("parseScoreResponse() error = %v", err)
	}
	if len(parsed.Scores) != 1 || parsed.Scores[0].ID != "m1" || parsed.Scores[0].Score != 0.7 {
		t.Fatalf("parsed = %#v", parsed)
	}
}

func TestSearchScoresThreadMessages(t *testing.T) {
	messageID := modeluuid.New()
	threadID := modeluuid.New()
	c := &Component{
		config:   ComponentConfig{Completion: "llm/qwen", BatchSize: 10, Limit: 5, MinScore: 0.4, MaxOutputTokens: 256},
		resolver: fakeResolver{provider: fakeCompletionProvider{reply: `{"scores":[{"id":"` + messageID.String() + `","score":0.7,"reason":"mentions ORM tradeoffs"}]}`}},
	}
	c.SetSearchMessageSource(fakeMessageSource{messages: []coremodel.ThreadMessage{{ID: messageID, ThreadID: threadID, Text: "GORM vs raw SQL"}}})
	response, err := c.Search(context.Background(), component.SearchRequest{Query: "database abstraction layer", ThreadID: threadID})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(response.Results) != 1 {
		t.Fatalf("results = %#v, want one result", response.Results)
	}
	result := response.Results[0]
	if result.MessageID != messageID || result.Score != 0.7 || !strings.Contains(result.Excerpt, "GORM") {
		t.Fatalf("result = %#v", result)
	}
}

func TestSearchKeepsCompletionSessionOpenAcrossBatches(t *testing.T) {
	threadID := modeluuid.New()
	messageID := modeluuid.New()
	provider := &fakeSessionCompletionProvider{
		reply: `{"scores":[{"id":"` + messageID.String() + `","score":0.7}]}`,
	}
	c := &Component{
		config:   ComponentConfig{Completion: "llm/qwen", BatchSize: 10, Limit: 5, MinScore: 0.4, MaxOutputTokens: 256},
		resolver: fakeResolver{provider: provider},
	}
	messages := []coremodel.ThreadMessage{{ID: messageID, ThreadID: threadID, Text: "first message"}}
	for i := 0; i < 24; i++ {
		messages = append(messages, coremodel.ThreadMessage{ID: modeluuid.New(), ThreadID: threadID, Text: "other message"})
	}
	c.SetSearchMessageSource(fakeMessageSource{messages: messages})
	if _, err := c.Search(context.Background(), component.SearchRequest{Query: "anything", ThreadID: threadID}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if provider.begins != 1 || provider.closes != 1 {
		t.Fatalf("session begins=%d closes=%d, want 1/1", provider.begins, provider.closes)
	}
	if provider.completions != 3 {
		t.Fatalf("completions=%d, want 3 batches", provider.completions)
	}
}

func TestSearchPassesCompletionIdleTimeoutToSession(t *testing.T) {
	threadID := modeluuid.New()
	messageID := modeluuid.New()
	provider := &fakeSessionCompletionProvider{
		reply: `{"scores":[{"id":"` + messageID.String() + `","score":0.7}]}`,
	}
	c := &Component{
		config:   ComponentConfig{Completion: "llm/qwen", BatchSize: 10, Limit: 5, MinScore: 0.4, MaxOutputTokens: 256},
		resolver: fakeResolver{provider: provider},
	}
	c.SetSearchMessageSource(fakeMessageSource{messages: []coremodel.ThreadMessage{{ID: messageID, ThreadID: threadID, Text: "message"}}})
	if _, err := c.Search(context.Background(), component.SearchRequest{Query: "anything", ThreadID: threadID, CompletionIdleTimeout: 10 * time.Second}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if provider.idleTimeout.String() != "10s" {
		t.Fatalf("idleTimeout=%s, want 10s", provider.idleTimeout)
	}
}

type fakeMessageSource struct{ messages []coremodel.ThreadMessage }

func (f fakeMessageSource) ThreadMessages(context.Context, modeluuid.UUID) ([]coremodel.ThreadMessage, error) {
	return f.messages, nil
}

type fakeCompletionProvider struct{ reply string }

func (f fakeCompletionProvider) Type() string { return "llm" }

func (f fakeCompletionProvider) HandleCompletion(context.Context, component.CompletionRequest) (*component.CompletionResult, error) {
	return &component.CompletionResult{Final: &coremodel.ThreadMessage{Text: f.reply}}, nil
}

type fakeSessionCompletionProvider struct {
	reply       string
	begins      int
	closes      int
	completions int
	idleTimeout time.Duration
}

func (f *fakeSessionCompletionProvider) Type() string { return "llm" }

func (f *fakeSessionCompletionProvider) HandleCompletion(context.Context, component.CompletionRequest) (*component.CompletionResult, error) {
	f.completions++
	return &component.CompletionResult{Final: &coremodel.ThreadMessage{Text: f.reply}}, nil
}

func (f *fakeSessionCompletionProvider) BeginCompletionSession(_ context.Context, options component.CompletionSessionOptions) (component.CompletionSession, error) {
	f.begins++
	f.idleTimeout = options.IdleTimeout
	return fakeCompletionSession{close: func() error {
		f.closes++
		return nil
	}}, nil
}

type fakeCompletionSession struct {
	close func() error
}

func (f fakeCompletionSession) Close() error {
	if f.close == nil {
		return nil
	}
	return f.close()
}

type fakeResolver struct{ provider component.CompletionProvider }

func (f fakeResolver) ResolveComponentRef(context.Context, string) (*coremodel.Component, error) {
	return &coremodel.Component{ID: modeluuid.New(), Type: "llm", Name: "qwen", Enabled: true}, nil
}

func (f fakeResolver) ResolveComponent(context.Context, modeluuid.UUID) (*component.Loaded, error) {
	return &component.Loaded{Registration: coremodel.Component{Type: "llm", Name: "qwen"}, Component: f.provider}, nil
}
