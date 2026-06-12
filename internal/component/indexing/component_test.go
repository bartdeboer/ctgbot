package indexing

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
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
	embedder := &fakeEmbeddingEngine{}
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

func TestEmbeddingStrategyKeepsInferenceSessionOpenAcrossBatches(t *testing.T) {
	ctx := context.Background()
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	messages := []coremodel.ThreadMessage{
		messageFixture(chatID, threadID, coremodel.MessageRoleUser, "first message"),
		messageFixture(chatID, threadID, coremodel.MessageRoleAgent, "second message"),
		messageFixture(chatID, threadID, coremodel.MessageRoleUser, "third message"),
	}
	embedder := &fakeEmbeddingEngine{}
	component := newTestComponent(t, newFakeResolver(map[string]component.Component{"llamacpp": embedder}), messages)
	if err := component.store.saveStrategy(ctx, &indexStrategy{Name: "default-message", Type: StrategyTypeEmbedding, ProviderRef: "llamacpp", Model: "qwen-embed", BatchSize: 2}); err != nil {
		t.Fatal(err)
	}

	if _, err := component.RunStrategy(ctx, RunRequest{Strategy: "default-message", Scope: scope{ThreadID: threadID}}); err != nil {
		t.Fatal(err)
	}
	if embedder.calls != 2 {
		t.Fatalf("embed calls = %d, want 2 batches", embedder.calls)
	}
	if len(embedder.sessionRequests) != 1 {
		t.Fatalf("session requests = %d, want 1", len(embedder.sessionRequests))
	}
	if embedder.sessionRequests[0].Model != "qwen-embed" || embedder.sessionRequests[0].IdleTimeout != indexingRunIdleTimeout {
		t.Fatalf("session request = %#v", embedder.sessionRequests[0])
	}
	if embedder.sessionCloseCalls != 1 {
		t.Fatalf("session close calls = %d, want 1", embedder.sessionCloseCalls)
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

func TestSummaryStrategyMaxMessagesAppliesAfterEligibilityFiltering(t *testing.T) {
	ctx := context.Background()
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	messages := []coremodel.ThreadMessage{
		messageFixture(chatID, threadID, coremodel.MessageRoleUser, "older user"),
		messageFixture(chatID, threadID, coremodel.MessageRoleSystem, "newer system 1"),
		messageFixture(chatID, threadID, coremodel.MessageRoleAgent, "newer agent"),
		messageFixture(chatID, threadID, coremodel.MessageRoleSystem, "newer system 2"),
		messageFixture(chatID, threadID, coremodel.MessageRoleUser, "newest user"),
	}
	completion := &fakeCompletion{}
	component := newTestComponent(t, newFakeResolver(map[string]component.Component{"llamacpp": completion}), messages)
	if err := component.store.saveStrategy(ctx, &indexStrategy{Name: "context-100", Type: StrategyTypeSummary, ProviderRef: "llamacpp", Model: "qwen3.5", TargetChars: 100}); err != nil {
		t.Fatal(err)
	}

	result, err := component.RunStrategy(ctx, RunRequest{Strategy: "context-100", Scope: scope{ThreadID: threadID}, MaxMessages: 2})
	if err != nil {
		t.Fatal(err)
	}
	if result.Messages != 2 || result.Created != 2 {
		t.Fatalf("result = %#v, want two eligible user/agent messages", result)
	}
}

func TestSummaryStrategyIndexesLegacyUserAgentKinds(t *testing.T) {
	ctx := context.Background()
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	messages := []coremodel.ThreadMessage{
		legacyMessageFixture(chatID, threadID, coremodel.MessageKind("user"), "legacy user"),
		legacyMessageFixture(chatID, threadID, coremodel.MessageKind("agent"), "legacy agent"),
		legacyMessageFixture(chatID, threadID, coremodel.MessageKind("system"), "legacy system"),
	}
	completion := &fakeCompletion{}
	component := newTestComponent(t, newFakeResolver(map[string]component.Component{"llamacpp": completion}), messages)
	if err := component.store.saveStrategy(ctx, &indexStrategy{Name: "context-100", Type: StrategyTypeSummary, ProviderRef: "llamacpp", Model: "qwen3.5", TargetChars: 100}); err != nil {
		t.Fatal(err)
	}

	result, err := component.RunStrategy(ctx, RunRequest{Strategy: "context-100", Scope: scope{All: true}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Messages != 2 || result.Created != 2 {
		t.Fatalf("result = %#v, want legacy user and agent only", result)
	}
}

func TestSummaryStrategyCopiesShortMessagesWithoutCompletion(t *testing.T) {
	ctx := context.Background()
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	messages := []coremodel.ThreadMessage{
		messageFixture(chatID, threadID, coremodel.MessageRoleUser, "short exact message"),
	}
	completion := &fakeCompletion{}
	component := newTestComponent(t, newFakeResolver(map[string]component.Component{"llamacpp": completion}), messages)
	if err := component.store.saveStrategy(ctx, &indexStrategy{Name: "search-title", Type: StrategyTypeSummary, ProviderRef: "llamacpp", Model: "qwen3", TargetChars: 80, CopyUnderChars: 80}); err != nil {
		t.Fatal(err)
	}

	result, err := component.RunStrategy(ctx, RunRequest{Strategy: "search-title", Scope: scope{All: true}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Messages != 1 || result.Created != 1 {
		t.Fatalf("result = %#v", result)
	}
	if completion.calls != 0 {
		t.Fatalf("completion calls = %d, want 0", completion.calls)
	}
	if len(completion.sessionRequests) != 0 {
		t.Fatalf("session requests = %d, want 0", len(completion.sessionRequests))
	}
	strategy, err := component.store.strategyByName(ctx, "search-title")
	if err != nil {
		t.Fatal(err)
	}
	summary, err := component.store.summary(ctx, strategy.ID, messages[0].ID.String())
	if err != nil {
		t.Fatal(err)
	}
	if summary == nil || summary.Summary != "short exact message" {
		t.Fatalf("summary = %#v, want verbatim text", summary)
	}
}

func TestSummaryStrategySummarizesLongMessages(t *testing.T) {
	ctx := context.Background()
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	messages := []coremodel.ThreadMessage{
		messageFixture(chatID, threadID, coremodel.MessageRoleUser, "this message is definitely longer than the tiny copy threshold"),
	}
	completion := &fakeCompletion{}
	component := newTestComponent(t, newFakeResolver(map[string]component.Component{"llamacpp": completion}), messages)
	if err := component.store.saveStrategy(ctx, &indexStrategy{Name: "search-title", Type: StrategyTypeSummary, ProviderRef: "llamacpp", Model: "qwen3", TargetChars: 80, CopyUnderChars: 10}); err != nil {
		t.Fatal(err)
	}

	if _, err := component.RunStrategy(ctx, RunRequest{Strategy: "search-title", Scope: scope{All: true}}); err != nil {
		t.Fatal(err)
	}
	if completion.calls != 1 {
		t.Fatalf("completion calls = %d, want 1", completion.calls)
	}
}

func TestSearchComponentEmbeddingSearchUsesIndexAndTitleStrategy(t *testing.T) {
	ctx := context.Background()
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	messages := []coremodel.ThreadMessage{
		messageFixture(chatID, threadID, coremodel.MessageRoleUser, "Docker containers keep runtime tools isolated."),
		messageFixture(chatID, threadID, coremodel.MessageRoleAgent, "Gmail OAuth credentials need refresh handling."),
	}
	embedder := &keywordEmbeddingEngine{}
	indexing := newTestComponent(t, newFakeResolver(map[string]component.Component{"llamacpp": embedder}), messages)
	if err := indexing.store.saveStrategy(ctx, &indexStrategy{Name: DefaultSearchEmbeddingStrategy, Type: StrategyTypeEmbedding, ProviderRef: "llamacpp", Model: "keyword-embed"}); err != nil {
		t.Fatal(err)
	}
	if err := indexing.store.saveStrategy(ctx, &indexStrategy{Name: DefaultSearchTitleStrategy, Type: StrategyTypeSummary, ProviderRef: "llamacpp", Model: "qwen", TargetChars: 80}); err != nil {
		t.Fatal(err)
	}
	if _, err := indexing.RunStrategy(ctx, RunRequest{Strategy: DefaultSearchEmbeddingStrategy, Scope: scope{All: true}}); err != nil {
		t.Fatal(err)
	}
	titleStrategy, err := indexing.store.strategyByName(ctx, DefaultSearchTitleStrategy)
	if err != nil {
		t.Fatal(err)
	}
	if err := indexing.store.saveSummary(ctx, messageSummary{MessageID: messages[0].ID.String(), StrategyID: titleStrategy.ID, ChatID: chatID.String(), ThreadID: threadID.String(), SourceHash: textHash(messages[0].Text), Summary: "Docker runtime isolation"}); err != nil {
		t.Fatal(err)
	}
	search := newTestSearchComponent(t, newFakeResolver(map[string]component.Component{"indexing": indexing, "llamacpp": embedder}), messages)

	results, err := search.Search(ctx, SearchRequest{
		Query:             "docker runtime",
		Mode:              searchModeEmbedding,
		IndexingComponent: DefaultIndexingComponentRef,
		EmbeddingStrategy: DefaultSearchEmbeddingStrategy,
		TitleStrategy:     DefaultSearchTitleStrategy,
		Scope:             scope{All: true},
		Limit:             1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].MessageID != messages[0].ID {
		t.Fatalf("top result = %s, want %s", results[0].MessageID, messages[0].ID)
	}
	if results[0].Title != "Docker runtime isolation" {
		t.Fatalf("title = %q", results[0].Title)
	}
}

func TestSearchComponentKeywordSearchDoesNotRequireEmbeddingStrategy(t *testing.T) {
	ctx := context.Background()
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	messages := []coremodel.ThreadMessage{
		messageFixture(chatID, threadID, coremodel.MessageRoleUser, "Docker containers keep runtime tools isolated."),
		messageFixture(chatID, threadID, coremodel.MessageRoleAgent, "Gmail OAuth credentials need refresh handling."),
	}
	indexing := newTestComponent(t, newFakeResolver(nil), messages)
	search := newTestSearchComponent(t, newFakeResolver(map[string]component.Component{"indexing": indexing}), messages)

	results, err := search.Search(ctx, SearchRequest{
		Query:             "oauth",
		Mode:              searchModeKeyword,
		IndexingComponent: DefaultIndexingComponentRef,
		TitleStrategy:     DefaultSearchTitleStrategy,
		Scope:             scope{All: true},
		Limit:             10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].MessageID != messages[1].ID {
		t.Fatalf("keyword result = %s, want %s", results[0].MessageID, messages[1].ID)
	}
}

func TestSearchComponentRoutesAsSearchQueryCommand(t *testing.T) {
	ctx := context.Background()
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	messages := []coremodel.ThreadMessage{
		messageFixture(chatID, threadID, coremodel.MessageRoleUser, "Gmail OAuth credentials need refresh handling."),
	}
	indexing := newTestComponent(t, newFakeResolver(nil), messages)
	search := newTestSearchComponent(t, newFakeResolver(map[string]component.Component{"indexing": indexing}), messages)
	engine, err := commandset.NewBoundEngineForSource(commandengine.SourceHostbridge, []commandset.BoundSurface{{
		Surface:       search,
		ComponentRef:  "search",
		ComponentType: SearchType,
	}})
	if err != nil {
		t.Fatal(err)
	}

	result, err := engine.Run(ctx, commandengine.Request{Context: commandengine.Context{
		Source: commandengine.SourceHostbridge,
		Actor:  commandengine.Actor{ID: "agent", Roles: []simplerbac.Role{simplerbac.RoleAgent}},
	}}, []string{"search", "oauth", "--mode", "keyword"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Text, "Gmail OAuth") {
		t.Fatalf("result text = %q", result.Text)
	}
}

func TestSummaryStrategyKeepsInferenceSessionOpenForRun(t *testing.T) {
	ctx := context.Background()
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	messages := []coremodel.ThreadMessage{
		messageFixture(chatID, threadID, coremodel.MessageRoleUser, "first long message that needs the model"),
		messageFixture(chatID, threadID, coremodel.MessageRoleAgent, "second long message that needs the model"),
	}
	completion := &fakeCompletion{}
	component := newTestComponent(t, newFakeResolver(map[string]component.Component{"llamacpp": completion}), messages)
	if err := component.store.saveStrategy(ctx, &indexStrategy{Name: "search-title", Type: StrategyTypeSummary, ProviderRef: "llamacpp", Model: "qwen3", TargetChars: 80}); err != nil {
		t.Fatal(err)
	}

	if _, err := component.RunStrategy(ctx, RunRequest{Strategy: "search-title", Scope: scope{All: true}}); err != nil {
		t.Fatal(err)
	}
	if completion.calls != 2 {
		t.Fatalf("completion calls = %d, want 2", completion.calls)
	}
	if len(completion.sessionRequests) != 1 {
		t.Fatalf("session requests = %d, want 1", len(completion.sessionRequests))
	}
	if completion.sessionRequests[0].Model != "qwen3" || completion.sessionRequests[0].IdleTimeout != indexingRunIdleTimeout {
		t.Fatalf("session request = %#v", completion.sessionRequests[0])
	}
	if completion.sessionCloseCalls != 1 {
		t.Fatalf("session close calls = %d, want 1", completion.sessionCloseCalls)
	}
}

func TestClearIndexKeepsStrategies(t *testing.T) {
	ctx := context.Background()
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	messages := []coremodel.ThreadMessage{
		messageFixture(chatID, threadID, coremodel.MessageRoleUser, "Index this message."),
	}
	embedder := &fakeEmbeddingEngine{}
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
	created, err := New(context.Background(), registration, nil, runtimepkg.Profile{Path: t.TempDir()}, nil, resolver, nil)
	if err != nil {
		t.Fatal(err)
	}
	component := created.(*Component)
	component.SetSearchMessageSource(fakeMessageSource{messages: messages})
	return component
}

func newTestSearchComponent(t *testing.T, resolver fakeResolver, messages []coremodel.ThreadMessage) *SearchComponent {
	t.Helper()
	registration := coremodel.Component{ID: modeluuid.New(), Type: SearchType, Name: SearchType}
	created, err := NewSearch(context.Background(), registration, nil, runtimepkg.Profile{Path: t.TempDir()}, nil, resolver, nil)
	if err != nil {
		t.Fatal(err)
	}
	component := created.(*SearchComponent)
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
	var filtered []coremodel.ThreadMessage
	for _, message := range items {
		if shouldVisit(message, scope) {
			filtered = append(filtered, message)
		}
	}
	if scope.Order == component.MessageOrderNewestFirst {
		for i := 0; i < len(filtered)/2; i++ {
			j := len(filtered) - 1 - i
			filtered[i], filtered[j] = filtered[j], filtered[i]
		}
	}
	if scope.Limit > 0 && len(filtered) > scope.Limit {
		filtered = filtered[:scope.Limit]
	}
	for _, message := range filtered {
		if err := visit(message); err != nil {
			return err
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
	if len(scope.Kinds) > 0 {
		allowed := false
		for _, kind := range scope.Kinds {
			if message.Kind == kind {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	if len(scope.Roles) > 0 {
		allowed := false
		role := message.Role
		if role == "" {
			switch message.Kind {
			case coremodel.MessageKind("user"):
				role = coremodel.MessageRoleUser
			case coremodel.MessageKind("agent"):
				role = coremodel.MessageRoleAgent
			}
		}
		for _, candidate := range scope.Roles {
			if role == candidate {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	return true
}

type fakeEmbeddingEngine struct {
	calls             int
	sessionRequests   []component.InferenceSessionOptions
	sessionCloseCalls int
}

func (f *fakeEmbeddingEngine) Type() string { return "fake-embedder" }
func (f *fakeEmbeddingEngine) Embed(ctx context.Context, req component.EmbeddingRequest) (component.EmbeddingResponse, error) {
	_ = ctx
	f.calls++
	out := make([]component.Embedding, 0, len(req.Inputs))
	for i, input := range req.Inputs {
		out = append(out, component.Embedding{ID: input.ID, Model: req.Model, Dim: 2, Normalized: true, Vector: []float32{float32(i + 1), float32(len(input.Text))}})
	}
	return component.EmbeddingResponse{Embeddings: out}, nil
}

func (f *fakeEmbeddingEngine) BeginInferenceSession(ctx context.Context, options component.InferenceSessionOptions) (component.InferenceSession, error) {
	_ = ctx
	f.sessionRequests = append(f.sessionRequests, options)
	return fakeInferenceSession{close: func() { f.sessionCloseCalls++ }}, nil
}

type keywordEmbeddingEngine struct{}

func (f *keywordEmbeddingEngine) Type() string { return "keyword-embedder" }
func (f *keywordEmbeddingEngine) Embed(ctx context.Context, req component.EmbeddingRequest) (component.EmbeddingResponse, error) {
	_ = ctx
	out := make([]component.Embedding, 0, len(req.Inputs))
	for _, input := range req.Inputs {
		out = append(out, component.Embedding{ID: input.ID, Model: req.Model, Dim: 2, Normalized: true, Vector: keywordVector(input.Text)})
	}
	return component.EmbeddingResponse{Embeddings: out}, nil
}

func keywordVector(text string) []float32 {
	text = strings.ToLower(text)
	switch {
	case strings.Contains(text, "docker") || strings.Contains(text, "runtime") || strings.Contains(text, "container"):
		return []float32{1, 0}
	case strings.Contains(text, "gmail") || strings.Contains(text, "oauth"):
		return []float32{0, 1}
	default:
		return []float32{0.5, 0.5}
	}
}

type fakeCompletion struct {
	lastPrompt        string
	calls             int
	sessionRequests   []component.InferenceSessionOptions
	sessionCloseCalls int
}

func (f *fakeCompletion) Type() string { return "fake-completion" }
func (f *fakeCompletion) Complete(ctx context.Context, req component.CompletionRequest) (*component.CompletionResult, error) {
	_ = ctx
	f.calls++
	if len(req.Prompt.Messages) > 0 {
		f.lastPrompt = req.Prompt.Messages[0].Content
	}
	return &component.CompletionResult{Final: &coremodel.ThreadMessage{Text: "summary: " + firstLine(f.lastPrompt)}}, nil
}

func (f *fakeCompletion) BeginInferenceSession(ctx context.Context, options component.InferenceSessionOptions) (component.InferenceSession, error) {
	_ = ctx
	f.sessionRequests = append(f.sessionRequests, options)
	return fakeInferenceSession{close: func() { f.sessionCloseCalls++ }}, nil
}

type fakeInferenceSession struct{ close func() }

func (s fakeInferenceSession) Close() error {
	if s.close != nil {
		s.close()
	}
	return nil
}

func messageFixture(chatID modeluuid.UUID, threadID modeluuid.UUID, role coremodel.MessageRole, text string) coremodel.ThreadMessage {
	return coremodel.ThreadMessage{ID: modeluuid.New(), ChatID: chatID, ThreadID: threadID, Role: role, Kind: coremodel.MessageKindMessage, Text: text}
}

func legacyMessageFixture(chatID modeluuid.UUID, threadID modeluuid.UUID, kind coremodel.MessageKind, text string) coremodel.ThreadMessage {
	return coremodel.ThreadMessage{ID: modeluuid.New(), ChatID: chatID, ThreadID: threadID, Kind: kind, Text: text}
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(text, "\n")
	return strings.TrimSpace(line)
}

type clirRequest struct{ params map[string]string }

func (r clirRequest) request(args ...string) *clir.Request {
	return &clir.Request{Params: r.params, Extra: args}
}
