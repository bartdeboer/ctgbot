package semantic

import (
	"context"
	"strings"
	"testing"

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

type fakeMessageSource struct{ messages []coremodel.ThreadMessage }

func (f fakeMessageSource) ThreadMessages(context.Context, modeluuid.UUID) ([]coremodel.ThreadMessage, error) {
	return f.messages, nil
}

type fakeCompletionProvider struct{ reply string }

func (f fakeCompletionProvider) Type() string { return "llm" }

func (f fakeCompletionProvider) HandleCompletion(context.Context, component.CompletionRequest) (*component.CompletionResult, error) {
	return &component.CompletionResult{Final: &coremodel.ThreadMessage{Text: f.reply}}, nil
}

type fakeResolver struct{ provider component.CompletionProvider }

func (f fakeResolver) ResolveComponentRef(context.Context, string) (*coremodel.Component, error) {
	return &coremodel.Component{ID: modeluuid.New(), Type: "llm", Name: "qwen", Enabled: true}, nil
}

func (f fakeResolver) ResolveComponent(context.Context, modeluuid.UUID) (*component.Loaded, error) {
	return &component.Loaded{Registration: coremodel.Component{Type: "llm", Name: "qwen"}, Component: f.provider}, nil
}
