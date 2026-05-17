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

func TestStrategyEmbeddingIndexAndSearch(t *testing.T) {
	threadID := modeluuid.New()
	horseID := modeluuid.New()
	databaseID := modeluuid.New()
	store, err := openStore(t.TempDir())
	if err != nil {
		t.Fatalf("openStore() error = %v", err)
	}
	embedder := fakeEmbedder{}
	c := &Component{
		config:   ComponentConfig{Limit: 5, EmbeddingBatchSize: 10},
		store:    store,
		resolver: fakeResolver{component: embedder},
	}
	c.SetSearchMessageSource(fakeMessageSource{messages: []coremodel.ThreadMessage{
		{ID: horseID, ThreadID: threadID, Text: "The black horse runs through the field."},
		{ID: databaseID, ThreadID: threadID, Text: "We discussed SQLite embeddings and semantic search strategies."},
	}})
	if err := store.saveStrategy(context.Background(), &strategy{
		Name:        "qwen-embed",
		Type:        strategyTypeEmbedding,
		SourceKind:  strategySourceMessages,
		EmbedderRef: "llamacpp/local",
		Model:       "qwen3-embed",
		Prompt:      defaultQueryInstruction,
	}); err != nil {
		t.Fatalf("saveStrategy() error = %v", err)
	}
	indexed, err := c.IndexThread(context.Background(), IndexRequest{Strategy: "qwen-embed", ThreadID: threadID})
	if err != nil {
		t.Fatalf("IndexThread() error = %v", err)
	}
	if indexed.Messages != 2 || indexed.Embedded != 2 || indexed.Skipped != 0 {
		t.Fatalf("indexed = %#v, want 2/2/0", indexed)
	}
	indexedAgain, err := c.IndexThread(context.Background(), IndexRequest{Strategy: "qwen-embed", ThreadID: threadID})
	if err != nil {
		t.Fatalf("IndexThread() second error = %v", err)
	}
	if indexedAgain.Embedded != 0 || indexedAgain.Skipped != 2 {
		t.Fatalf("indexedAgain = %#v, want skipped existing embeddings", indexedAgain)
	}
	results, err := c.SearchStrategy(context.Background(), StrategySearchRequest{Strategy: "qwen-embed", ThreadID: threadID, Query: "database search", Limit: 1})
	if err != nil {
		t.Fatalf("SearchStrategy() error = %v", err)
	}
	if len(results.Results) != 1 {
		t.Fatalf("results = %#v, want one", results.Results)
	}
	if results.Results[0].MessageID != databaseID {
		t.Fatalf("top result = %s, want database message %s", results.Results[0].MessageID, databaseID)
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

type fakeEmbedder struct{}

func (fakeEmbedder) Type() string { return "embedder" }

func (fakeEmbedder) Embed(_ context.Context, req component.EmbedRequest) (component.EmbedResponse, error) {
	out := make([]component.Embedding, 0, len(req.Inputs))
	for _, input := range req.Inputs {
		out = append(out, component.Embedding{
			ID:         input.ID,
			Vector:     fakeVector(input.Text),
			Dim:        3,
			Model:      req.Model,
			Normalized: true,
		})
	}
	return component.EmbedResponse{Embeddings: out}, nil
}

func fakeVector(text string) []float32 {
	text = strings.ToLower(text)
	var out [3]float32
	if strings.Contains(text, "horse") {
		out[0] = 1
	}
	if strings.Contains(text, "database") || strings.Contains(text, "sqlite") || strings.Contains(text, "search") {
		out[1] = 1
	}
	if strings.Contains(text, "email") || strings.Contains(text, "gmail") {
		out[2] = 1
	}
	return out[:]
}

type fakeResolver struct {
	provider  component.CompletionProvider
	component component.Component
}

func (f fakeResolver) ResolveComponentRef(context.Context, string) (*coremodel.Component, error) {
	return &coremodel.Component{ID: modeluuid.New(), Type: "llm", Name: "qwen", Enabled: true}, nil
}

func (f fakeResolver) ResolveComponent(context.Context, modeluuid.UUID) (*component.Loaded, error) {
	if f.component != nil {
		return &component.Loaded{Registration: coremodel.Component{Type: f.component.Type(), Name: "local"}, Component: f.component}, nil
	}
	return &component.Loaded{Registration: coremodel.Component{Type: "llm", Name: "qwen"}, Component: f.provider}, nil
}
