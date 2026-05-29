package indexing

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/go-clir"
)

func TestEmbeddingStrategyRunIndexesMessagesAndSkipsUnchanged(t *testing.T) {
	ctx := context.Background()
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	messages := []coremodel.ThreadMessage{
		messageFixture(chatID, threadID, coremodel.MessageRoleUser, "We discussed indexing message embeddings."),
		messageFixture(chatID, threadID, coremodel.MessageRoleAgent, "The agent proposed summary strategies."),
		messageFixture(chatID, threadID, coremodel.MessageRoleSystem, "system message should not be indexed"),
	}
	embedder := &fakeEmbedder{}
	component := newTestComponent(t, newFakeResolver(map[string]component.Component{"llamacpp": embedder}), messages)
	if err := component.store.saveStrategy(ctx, &indexStrategy{Name: "default-message", Type: StrategyTypeEmbedding, ProviderRef: "llamacpp", Model: "qwen-embed"}); err != nil {
		t.Fatal(err)
	}

	first, err := component.RunStrategy(ctx, RunRequest{Strategy: "default-message", Scope: scope{ThreadID: threadID}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Messages != 2 || first.Created != 2 || first.Updated != 0 || first.Skipped != 0 {
		t.Fatalf("first run = %#v", first)
	}
	if embedder.calls != 1 {
		t.Fatalf("embedder calls = %d, want 1", embedder.calls)
	}

	second, err := component.RunStrategy(ctx, RunRequest{Strategy: "default-message", Scope: scope{ThreadID: threadID}})
	if err != nil {
		t.Fatal(err)
	}
	if second.Messages != 2 || second.Created != 0 || second.Updated != 0 || second.Skipped != 2 {
		t.Fatalf("second run = %#v", second)
	}
	if embedder.calls != 1 {
		t.Fatalf("embedder calls after skip = %d, want 1", embedder.calls)
	}
}

func TestSummaryStrategyRunStoresPerMessageSummaries(t *testing.T) {
	ctx := context.Background()
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	messages := []coremodel.ThreadMessage{
		messageFixture(chatID, threadID, coremodel.MessageRoleUser, "Please summarize this message."),
	}
	completion := &fakeCompletion{}
	component := newTestComponent(t, newFakeResolver(map[string]component.Component{"llamacpp": completion}), messages)
	if err := component.store.saveStrategy(ctx, &indexStrategy{Name: "context-100", Type: StrategyTypeSummary, ProviderRef: "llamacpp", Model: "qwen3.5", TargetChars: 100}); err != nil {
		t.Fatal(err)
	}

	result, err := component.RunStrategy(ctx, RunRequest{Strategy: "context-100", Scope: scope{ThreadID: threadID}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Messages != 1 || result.Created != 1 || result.Skipped != 0 {
		t.Fatalf("result = %#v", result)
	}
	strategy, err := component.store.strategyByName(ctx, "context-100")
	if err != nil {
		t.Fatal(err)
	}
	summary, err := component.store.summary(ctx, strategy.ID, messages[0].ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if summary == nil || !strings.Contains(summary.Summary, "summary:") {
		t.Fatalf("summary = %#v", summary)
	}
	if !strings.Contains(completion.lastPrompt, "Target: at most 100 characters") {
		t.Fatalf("prompt missing target chars: %s", completion.lastPrompt)
	}
}

func TestClearIndexKeepsStrategies(t *testing.T) {
	ctx := context.Background()
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	messages := []coremodel.ThreadMessage{
		messageFixture(chatID, threadID, coremodel.MessageRoleUser, "Index this message."),
	}
	embedder := &fakeEmbedder{}
	component := newTestComponent(t, newFakeResolver(map[string]component.Component{"llamacpp": embedder}), messages)
	if err := component.store.saveStrategy(ctx, &indexStrategy{Name: "default-message", Type: StrategyTypeEmbedding, ProviderRef: "llamacpp", Model: "qwen-embed"}); err != nil {
		t.Fatal(err)
	}
	if _, err := component.RunStrategy(ctx, RunRequest{Strategy: "default-message", Scope: scope{ThreadID: threadID}}); err != nil {
		t.Fatal(err)
	}

	result, err := component.store.clearStrategy(ctx, "default-message")
	if err != nil {
		t.Fatal(err)
	}
	if result.Strategy != "default-message" || result.Runs != 1 || result.Embeddings != 1 || result.Summaries != 0 {
		t.Fatalf("clear result = %#v", result)
	}
	stats, err := component.store.stats(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Strategies != 1 || stats.Runs != 0 || stats.Embeddings != 0 || stats.Summaries != 0 {
		t.Fatalf("stats = %#v, want strategy only", stats)
	}
}

func TestCommandBuildersRequireProviderAndModel(t *testing.T) {
	req := &clirRequest{params: map[string]string{"name": "context-100"}}
	_, err := buildStrategyAddSummaryCommand(req.request("--target-chars", "100"))
	if err == nil || !strings.Contains(err.Error(), "missing --completion") {
		t.Fatalf("summary err = %v", err)
	}
	_, err = buildStrategyAddEmbeddingCommand(req.request("--model", "qwen"))
	if err == nil || !strings.Contains(err.Error(), "missing --embedder") {
		t.Fatalf("embedding err = %v", err)
	}
}

func newTestComponent(t *testing.T, resolver fakeResolver, messages []coremodel.ThreadMessage) *Component {
	t.Helper()
	registration := coremodel.Component{ID: modeluuid.New(), Type: Type, Name: Type}
	created, err := New(context.Background(), registration, nil, runtimepkg.Home{Path: t.TempDir()}, nil, resolver, nil)
	if err != nil {
		t.Fatal(err)
	}
	component := created.(*Component)
	component.SetSearchMessageSource(fakeMessageSource{messages: messages})
	return component
}

type fakeResolver struct {
	byRef map[string]component.Component
	byID  map[modeluuid.UUID]string
}

func newFakeResolver(items map[string]component.Component) fakeResolver {
	byID := make(map[modeluuid.UUID]string, len(items))
	byRef := make(map[string]component.Component, len(items))
	for ref, candidate := range items {
		id := modeluuid.New()
		byRef[ref] = candidate
		byID[id] = ref
	}
	return fakeResolver{byRef: byRef, byID: byID}
}

func (r fakeResolver) ResolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error) {
	_ = ctx
	if _, ok := r.byRef[ref]; !ok {
		return nil, fmt.Errorf("not found: %s", ref)
	}
	for id, idRef := range r.byID {
		if idRef == ref {
			return &coremodel.Component{ID: id, Type: ref, Name: ref}, nil
		}
	}
	return nil, fmt.Errorf("missing fake id for ref: %s", ref)
}

func (r fakeResolver) ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*component.Loaded, error) {
	_ = ctx
	ref, ok := r.byID[componentID]
	if !ok {
		return nil, nil
	}
	candidate, ok := r.byRef[ref]
	if !ok {
		return nil, nil
	}
	return &component.Loaded{Registration: coremodel.Component{ID: componentID, Type: ref, Name: ref}, Component: candidate}, nil
}

type fakeMessageSource struct{ messages []coremodel.ThreadMessage }

func (s fakeMessageSource) ForEachMessage(ctx context.Context, scope component.MessageScope, visit component.MessageVisitor) error {
	_ = ctx
	items := s.messages
	if scope.Order == component.MessageOrderNewestFirst {
		for i := len(items) - 1; i >= 0; i-- {
			if shouldVisit(items[i], scope) {
				if err := visit(items[i]); err != nil {
					return err
				}
			}
		}
		return nil
	}
	for _, message := range items {
		if shouldVisit(message, scope) {
			if err := visit(message); err != nil {
				return err
			}
		}
	}
	return nil
}

func shouldVisit(message coremodel.ThreadMessage, scope component.MessageScope) bool {
	if !scope.All {
		if !scope.ThreadID.IsNull() && message.ThreadID != scope.ThreadID {
			return false
		}
		if !scope.ChatID.IsNull() && message.ChatID != scope.ChatID {
			return false
		}
	}
	return true
}

type fakeEmbedder struct{ calls int }

func (f *fakeEmbedder) Type() string { return "fake-embedder" }
func (f *fakeEmbedder) Embed(ctx context.Context, req component.EmbedRequest) (component.EmbedResponse, error) {
	_ = ctx
	f.calls++
	out := make([]component.Embedding, 0, len(req.Inputs))
	for i, input := range req.Inputs {
		out = append(out, component.Embedding{ID: input.ID, Model: req.Model, Dim: 2, Normalized: true, Vector: []float32{float32(i + 1), float32(len(input.Text))}})
	}
	return component.EmbedResponse{Embeddings: out}, nil
}

type fakeCompletion struct{ lastPrompt string }

func (f *fakeCompletion) Type() string { return "fake-completion" }
func (f *fakeCompletion) HandleCompletion(ctx context.Context, req component.CompletionRequest) (*component.CompletionResult, error) {
	_ = ctx
	if len(req.Prompt.Messages) > 0 {
		f.lastPrompt = req.Prompt.Messages[0].Content
	}
	return &component.CompletionResult{Final: &coremodel.ThreadMessage{Text: "summary: " + firstLine(f.lastPrompt)}}, nil
}

func messageFixture(chatID modeluuid.UUID, threadID modeluuid.UUID, role coremodel.MessageRole, text string) coremodel.ThreadMessage {
	return coremodel.ThreadMessage{ID: modeluuid.New(), ChatID: chatID, ThreadID: threadID, Role: role, Kind: coremodel.MessageKindMessage, Text: text}
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return strings.TrimSpace(line)
}

type clirRequest struct{ params map[string]string }

func (r clirRequest) request(args ...string) *clir.Request {
	return &clir.Request{Params: r.params, Extra: args}
}
