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
	"github.com/bartdeboer/ctgbot/internal/configsurface"
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
	Scope       scopeFlags
}

type indexDropCommand struct {
	Strategy string
	Scope    scopeFlags
}

type strategySearchCommand struct {
	Strategy    string
	Query       string
	Limit       int
	ExcerptSize int
	Full        bool
	Scope       scopeFlags
}

type statsCommand struct{}

func RegisterGobTypes(register func(any)) {
	register(strategyListCommand{})
	register(strategyAddEmbeddingCommand{})
	register(indexCommand{})
	register(indexDropCommand{})
	register(strategySearchCommand{})
	register(statsCommand{})
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) CommandDefinitions() []commandengine.Definition {
	policy := simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent)
	definitions := []commandengine.Definition{
		{
			Pattern: "strategy list",
			Help:    "List search strategies",
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
			Pattern: "index create <strategy>",
			Help:    "Index current thread messages for a strategy",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  policy,
			Build:   buildIndexCommand,
		},
		{
			Pattern: "index drop <strategy>",
			Help:    "Drop indexed vectors for a strategy",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  policy,
			Build:   buildIndexDropCommand,
		},
		{
			Pattern:               "search <query>",
			Help:                  "Search current thread history",
			Sources:               []commandengine.Source{commandengine.SourceHostbridge},
			Policy:                policy,
			InstructionVisibility: commandengine.InstructionImportant,
			Build:                 buildDefaultSearchCommand,
		},
		{
			Pattern: "strategy <strategy> search <query>",
			Help:    "Search current thread history using a search strategy",
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
	definitions = append(definitions, configsurface.CommandDefinitions(configsurface.DefinitionOptions{
		Sources:       []commandengine.Source{commandengine.SourceHostbridge},
		Policy:        policy,
		SupportsUnset: true,
	})...)
	return definitions
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
	var scope scopeFlags
	bindScopeFlags(fs, &scope)
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
	return indexCommand{Strategy: strategy, MaxMessages: *maxMessages, BatchSize: *batchSize, Scope: scope}, nil
}

func buildIndexDropCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("semantic index drop", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var scope scopeFlags
	bindScopeFlags(fs, &scope)
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected index drop arguments: %s", strings.Join(fs.Args(), " "))
	}
	strategy := strings.TrimSpace(req.Params["strategy"])
	if strategy == "" {
		return nil, fmt.Errorf("missing strategy")
	}
	return indexDropCommand{Strategy: strategy, Scope: scope}, nil
}

func buildStrategySearchCommand(req *clir.Request) (any, error) {
	return buildSearchCommand(req, true)
}

func buildDefaultSearchCommand(req *clir.Request) (any, error) {
	return buildSearchCommand(req, false)
}

func buildSearchCommand(req *clir.Request, requireStrategy bool) (any, error) {
	fs := flag.NewFlagSet("semantic search", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	limit := fs.Int("limit", 0, "Maximum results")
	excerptSize := fs.Int("excerpt-size", 0, "Maximum excerpt characters")
	full := fs.Bool("full", false, "Render full matched message text")
	var scope scopeFlags
	bindScopeFlags(fs, &scope)
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected search arguments: %s", strings.Join(fs.Args(), " "))
	}
	cmd := strategySearchCommand{Strategy: strings.TrimSpace(req.Params["strategy"]), Query: strings.TrimSpace(req.Params["query"]), Limit: *limit, ExcerptSize: *excerptSize, Full: *full, Scope: scope}
	if requireStrategy && cmd.Strategy == "" {
		return nil, fmt.Errorf("missing strategy")
	}
	if cmd.Query == "" {
		return nil, fmt.Errorf("missing search query")
	}
	return cmd, nil
}

func bindScopeFlags(fs *flag.FlagSet, scope *scopeFlags) {
	fs.StringVar(&scope.Chat, "chat", "", "Scope to a chat id")
	fs.StringVar(&scope.Thread, "thread", "", "Scope to a thread id")
	fs.BoolVar(&scope.All, "all", false, "Use all indexed messages")
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	handlers := []error{
		commandengine.RegisterPattern[strategyListCommand](registry, "strategy list", c.handleStrategyList),
		commandengine.RegisterPattern[strategyAddEmbeddingCommand](registry, "strategy add embedding <name>", c.handleStrategyAddEmbedding),
		commandengine.RegisterPattern[indexCommand](registry, "index create <strategy>", c.handleIndex),
		commandengine.RegisterPattern[indexDropCommand](registry, "index drop <strategy>", c.handleIndexDrop),
		commandengine.RegisterPattern[strategySearchCommand](registry, "search <query>", c.handleStrategySearch),
		commandengine.RegisterPattern[strategySearchCommand](registry, "strategy <strategy> search <query>", c.handleStrategySearch),
		commandengine.RegisterPattern[statsCommand](registry, "stats", c.handleStats),
		configsurface.RegisterCommandHandlers(registry, c),
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
	record := strategy{
		Name:        cmd.Name,
		Type:        strategyTypeEmbedding,
		SourceKind:  firstNonEmpty(cmd.SourceKind, strategySourceMessages),
		EmbedderRef: cmd.Embedder,
		Model:       cmd.Model,
		Prompt:      cmd.Prompt,
		BatchSize:   cmd.BatchSize,
	}
	if record.BatchSize <= 0 {
		record.BatchSize = c.config.EmbeddingBatchSize
	}
	if record.BatchSize <= 0 {
		record.BatchSize = DefaultEmbeddingBatchSize
	}
	if err := c.store.saveStrategy(ctx, &record); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("semantic strategy saved: %s", record.Name)}, nil
}

func (c *Component) handleIndex(ctx context.Context, req commandengine.Request, cmd indexCommand) (commandengine.Result, error) {
	scope, err := resolveIndexScope(req.Context, cmd.Scope)
	if err != nil {
		return commandengine.Result{}, err
	}
	started := time.Now()
	result, err := c.Index(ctx, IndexRequest{Strategy: cmd.Strategy, Scope: scope, MaxMessages: cmd.MaxMessages, BatchSize: cmd.BatchSize})
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("semantic index %s\nscope: %s\nmessages: %d\nembedded: %d\nskipped: %d\nelapsed: %s", cmd.Strategy, scopeText(scope), result.Messages, result.Embedded, result.Skipped, time.Since(started).Round(100*time.Millisecond))}, nil
}

func (c *Component) handleIndexDrop(ctx context.Context, req commandengine.Request, cmd indexDropCommand) (commandengine.Result, error) {
	scope, err := resolveScope(req.Context, cmd.Scope)
	if err != nil {
		return commandengine.Result{}, err
	}
	deleted, err := c.store.deleteEmbeddings(ctx, cmd.Strategy, scope)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("semantic index dropped: %s\nscope: %s\nembeddings: %d", cmd.Strategy, scopeText(scope), deleted)}, nil
}

func (c *Component) handleStrategySearch(ctx context.Context, req commandengine.Request, cmd strategySearchCommand) (commandengine.Result, error) {
	scope, err := resolveScope(req.Context, cmd.Scope)
	if err != nil {
		return commandengine.Result{}, err
	}
	strategyName, err := c.resolveSearchStrategy(ctx, cmd.Strategy)
	if err != nil {
		return commandengine.Result{}, err
	}
	options, err := searchOutputOptions(c.config, cmd)
	if err != nil {
		return commandengine.Result{}, err
	}
	started := time.Now()
	response, err := c.SearchStrategy(ctx, StrategySearchRequest{Strategy: strategyName, Query: cmd.Query, Scope: scope, Limit: options.Limit})
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: renderSearchResponse(strategyName+": "+cmd.Query, time.Since(started), response, options)}, nil
}

func (c *Component) resolveSearchStrategy(ctx context.Context, requested string) (string, error) {
	requested = normalizeStrategyName(requested)
	if requested != "" {
		return requested, nil
	}
	if c == nil || c.store == nil {
		return "", fmt.Errorf("missing semantic store")
	}
	strategies, err := c.store.listStrategies(ctx)
	if err != nil {
		return "", err
	}
	var candidates []string
	for _, strategy := range strategies {
		if strategy.Enabled && strategy.Type == strategyTypeEmbedding {
			candidates = append(candidates, strategy.Name)
		}
	}
	switch len(candidates) {
	case 0:
		return "", fmt.Errorf("no semantic search strategy configured; create one with semantic strategy add embedding <name>")
	case 1:
		return candidates[0], nil
	default:
		return "", fmt.Errorf("multiple semantic search strategies configured; use semantic strategy <strategy> search <query> with one of: %s", strings.Join(candidates, ", "))
	}
}

func (c *Component) handleStats(ctx context.Context, req commandengine.Request, cmd statsCommand) (commandengine.Result, error) {
	stats, err := c.store.stats(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("semantic stats\nstrategies: %d\nmessages: %d\nderivations: %d\nembeddings: %d", stats.Strategies, stats.Messages, stats.Derivations, stats.Embeddings)}, nil
}

type searchOutput struct {
	Limit       int
	ExcerptSize int
	Full        bool
}

func searchOutputOptions(config ComponentConfig, cmd strategySearchCommand) (searchOutput, error) {
	excerptSize := cmd.ExcerptSize
	if excerptSize < 0 {
		return searchOutput{}, fmt.Errorf("--excerpt-size must not be negative")
	}
	if excerptSize == 0 {
		excerptSize = DefaultExcerptSize
	}
	if excerptSize > MaxExcerptSize {
		return searchOutput{}, fmt.Errorf("--excerpt-size must be <= %d", MaxExcerptSize)
	}
	limit := cmd.Limit
	if limit < 0 {
		return searchOutput{}, fmt.Errorf("--limit must not be negative")
	}
	if cmd.Full {
		if limit <= 0 {
			limit = DefaultFullSearchResults
		}
		if limit > MaxFullSearchResults {
			return searchOutput{}, fmt.Errorf("--full supports at most %d results", MaxFullSearchResults)
		}
		return searchOutput{Limit: limit, ExcerptSize: excerptSize, Full: true}, nil
	}
	if limit <= 0 {
		limit = config.Limit
	}
	if limit <= 0 {
		limit = DefaultLimit
	}
	if limit > MaxSearchResults {
		return searchOutput{}, fmt.Errorf("--limit must be <= %d", MaxSearchResults)
	}
	return searchOutput{Limit: limit, ExcerptSize: excerptSize}, nil
}

func renderSearchResponse(query string, elapsed time.Duration, response component.SearchResponse, options searchOutput) string {
	header := fmt.Sprintf("semantic search: %s\nelapsed: %s", query, elapsed.Round(100*time.Millisecond))
	if len(response.Results) == 0 {
		return header + "\n(no results)"
	}
	var lines []string
	lines = append(lines, header)
	for i, result := range response.Results {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("%d. message_id=%s thread_id=%s score=%.3f", i+1, result.MessageID, result.ThreadID, result.Score))
		if options.Full {
			text := firstNonEmpty(result.Text, result.Excerpt)
			if strings.TrimSpace(text) != "" {
				lines = append(lines, "text:")
				lines = append(lines, fencedText(text))
			}
		} else {
			text := firstNonEmpty(result.Text, result.Excerpt)
			if strings.TrimSpace(text) != "" {
				lines = append(lines, "excerpt: "+excerpt(text, options.ExcerptSize))
			}
		}
		if strings.TrimSpace(result.Reason) != "" {
			lines = append(lines, "reason: "+result.Reason)
		}
	}
	return strings.Join(lines, "\n")
}

func fencedText(text string) string {
	text = strings.TrimRight(strings.TrimSpace(text), "\n")
	fence := "```"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	return fence + "text\n" + text + "\n" + fence
}
