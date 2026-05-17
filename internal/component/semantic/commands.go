package semantic

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type strategyListCommand struct{}

type strategyAddEmbeddingCommand struct {
	Name       string
	Embedder   string
	Model      string
	Prompt     string
	BatchSize  int
	SourceKind string
}

type indexCommand struct {
	Strategy    string
	MaxMessages int
	BatchSize   int
}

type strategySearchCommand struct {
	Strategy string
	Query    string
	Limit    int
}

type statsCommand struct{}

func RegisterGobTypes(register func(any)) {
	register(strategyListCommand{})
	register(strategyAddEmbeddingCommand{})
	register(indexCommand{})
	register(strategySearchCommand{})
	register(statsCommand{})
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) CommandDefinitions() []commandengine.Definition {
	policy := simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent)
	return []commandengine.Definition{
		{
			Pattern: "strategy list",
			Help:    "List semantic search strategies",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  policy,
			Build:   func(*clir.Request) (any, error) { return strategyListCommand{}, nil },
		},
		{
			Pattern: "strategy add embedding <name>",
			Help:    "Create an embedding search strategy",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  policy,
			Build:   buildStrategyAddEmbeddingCommand,
		},
		{
			Pattern: "index <strategy>",
			Help:    "Index current thread messages for a strategy",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  policy,
			Build:   buildIndexCommand,
		},
		{
			Pattern: "search <strategy> <query>",
			Help:    "Search current thread history using a semantic strategy",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  policy,
			Build:   buildStrategySearchCommand,
		},
		{
			Pattern: "stats",
			Help:    "Show semantic search index stats",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  policy,
			Build:   func(*clir.Request) (any, error) { return statsCommand{}, nil },
		},
	}
}

func buildStrategyAddEmbeddingCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("semantic strategy add embedding", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	embedder := fs.String("embedder", "", "Embedding component ref, for example llamacpp/local")
	model := fs.String("model", "", "Embedding model name")
	prompt := fs.String("prompt", defaultQueryInstruction, "Query instruction for embedding models that support it")
	batchSize := fs.Int("batch-size", 0, "Embedding batch size")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected strategy arguments: %s", strings.Join(fs.Args(), " "))
	}
	cmd := strategyAddEmbeddingCommand{
		Name:       strings.TrimSpace(req.Params["name"]),
		Embedder:   strings.TrimSpace(*embedder),
		Model:      strings.TrimSpace(*model),
		Prompt:     strings.TrimSpace(*prompt),
		BatchSize:  *batchSize,
		SourceKind: strategySourceMessages,
	}
	if cmd.Name == "" {
		return nil, fmt.Errorf("missing strategy name")
	}
	if cmd.Embedder == "" {
		return nil, fmt.Errorf("missing --embedder")
	}
	if cmd.Model == "" {
		return nil, fmt.Errorf("missing --model")
	}
	return cmd, nil
}

func buildIndexCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("semantic index", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	maxMessages := fs.Int("max-messages", 0, "Maximum recent messages to index")
	batchSize := fs.Int("batch-size", 0, "Embedding batch size")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected index arguments: %s", strings.Join(fs.Args(), " "))
	}
	strategy := strings.TrimSpace(req.Params["strategy"])
	if strategy == "" {
		return nil, fmt.Errorf("missing strategy")
	}
	return indexCommand{Strategy: strategy, MaxMessages: *maxMessages, BatchSize: *batchSize}, nil
}

func buildStrategySearchCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("semantic search", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	limit := fs.Int("limit", 0, "Maximum results")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected search arguments: %s", strings.Join(fs.Args(), " "))
	}
	cmd := strategySearchCommand{Strategy: strings.TrimSpace(req.Params["strategy"]), Query: strings.TrimSpace(req.Params["query"]), Limit: *limit}
	if cmd.Strategy == "" {
		return nil, fmt.Errorf("missing strategy")
	}
	if cmd.Query == "" {
		return nil, fmt.Errorf("missing search query")
	}
	return cmd, nil
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	handlers := []error{
		commandengine.RegisterPattern[strategyListCommand](registry, "strategy list", c.handleStrategyList),
		commandengine.RegisterPattern[strategyAddEmbeddingCommand](registry, "strategy add embedding <name>", c.handleStrategyAddEmbedding),
		commandengine.RegisterPattern[indexCommand](registry, "index <strategy>", c.handleIndex),
		commandengine.RegisterPattern[strategySearchCommand](registry, "search <strategy> <query>", c.handleStrategySearch),
		commandengine.RegisterPattern[statsCommand](registry, "stats", c.handleStats),
	}
	for _, err := range handlers {
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *Component) handleStrategyList(ctx context.Context, req commandengine.Request, cmd strategyListCommand) (commandengine.Result, error) {
	strategies, err := c.store.listStrategies(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(strategies) == 0 {
		return commandengine.Result{Text: "no semantic strategies"}, nil
	}
	var lines []string
	for _, strategy := range strategies {
		line := fmt.Sprintf("%s type=%s source=%s", strategy.Name, strategy.Type, strategy.SourceKind)
		if strategy.EmbedderRef != "" {
			line += " embedder=" + strategy.EmbedderRef
		}
		if strategy.Model != "" {
			line += " model=" + strategy.Model
		}
		lines = append(lines, line)
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) handleStrategyAddEmbedding(ctx context.Context, req commandengine.Request, cmd strategyAddEmbeddingCommand) (commandengine.Result, error) {
	strategy := semanticStrategy{
		Name:        cmd.Name,
		Type:        strategyTypeEmbedding,
		SourceKind:  firstNonEmpty(cmd.SourceKind, strategySourceMessages),
		EmbedderRef: cmd.Embedder,
		Model:       cmd.Model,
		Prompt:      cmd.Prompt,
		BatchSize:   cmd.BatchSize,
	}
	if strategy.BatchSize <= 0 {
		strategy.BatchSize = c.config.EmbeddingBatchSize
	}
	if strategy.BatchSize <= 0 {
		strategy.BatchSize = DefaultEmbeddingBatchSize
	}
	if err := c.store.saveStrategy(ctx, &strategy); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("semantic strategy saved: %s", strategy.Name)}, nil
}

func (c *Component) handleIndex(ctx context.Context, req commandengine.Request, cmd indexCommand) (commandengine.Result, error) {
	threadID := req.Context.ThreadID
	if threadID.IsNull() {
		threadID = req.Context.SandboxID
	}
	if threadID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("missing thread id")
	}
	started := time.Now()
	result, err := c.IndexThread(ctx, IndexRequest{Strategy: cmd.Strategy, ThreadID: threadID, MaxMessages: cmd.MaxMessages, BatchSize: cmd.BatchSize})
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("semantic index %s\nmessages: %d\nembedded: %d\nskipped: %d\nelapsed: %s", cmd.Strategy, result.Messages, result.Embedded, result.Skipped, time.Since(started).Round(100*time.Millisecond))}, nil
}

func (c *Component) handleStrategySearch(ctx context.Context, req commandengine.Request, cmd strategySearchCommand) (commandengine.Result, error) {
	threadID := req.Context.ThreadID
	if threadID.IsNull() {
		threadID = req.Context.SandboxID
	}
	if threadID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("missing thread id")
	}
	limit := cmd.Limit
	if limit <= 0 {
		limit = c.config.Limit
	}
	started := time.Now()
	response, err := c.SearchStrategy(ctx, StrategySearchRequest{Strategy: cmd.Strategy, Query: cmd.Query, ThreadID: threadID, Limit: limit})
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: c.renderSearchResponse(cmd.Strategy+": "+cmd.Query, time.Since(started), response)}, nil
}

func (c *Component) handleStats(ctx context.Context, req commandengine.Request, cmd statsCommand) (commandengine.Result, error) {
	stats, err := c.store.stats(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("semantic stats\nstrategies: %d\nmessages: %d\nderivations: %d\nembeddings: %d", stats.Strategies, stats.Messages, stats.Derivations, stats.Embeddings)}, nil
}

func (c *Component) renderSearchResponse(query string, elapsed time.Duration, response component.SearchResponse) string {
	header := fmt.Sprintf("semantic search: %s\nelapsed: %s", query, elapsed.Round(100*time.Millisecond))
	if len(response.Results) == 0 {
		return header + "\n(no results)"
	}
	var lines []string
	lines = append(lines, header)
	for i, result := range response.Results {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("%d. message_id=%s thread_id=%s score=%.3f", i+1, result.MessageID, result.ThreadID, result.Score))
		if strings.TrimSpace(result.Excerpt) != "" {
			lines = append(lines, "excerpt: "+result.Excerpt)
		}
		if strings.TrimSpace(result.Reason) != "" {
			lines = append(lines, "reason: "+result.Reason)
		}
	}
	return strings.Join(lines, "\n")
}
