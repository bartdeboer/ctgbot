package indexing

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type strategyListCommand struct{}

type strategyAddSummaryCommand struct {
	Name        string
	Completion  string
	Model       string
	Prompt      string
	TargetChars int
	BatchSize   int
}

type strategyAddEmbeddingCommand struct {
	Name      string
	Embedder  string
	Model     string
	BatchSize int
}

type runCommand struct {
	Strategy    string
	MaxMessages int
	BatchSize   int
	Scope       scopeFlags
}

type statsCommand struct{}

func RegisterGobTypes(register func(any)) {
	register(strategyListCommand{})
	register(strategyAddSummaryCommand{})
	register(strategyAddEmbeddingCommand{})
	register(runCommand{})
	register(statsCommand{})
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	policy := simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent)
	return []commandengine.Definition{
		{
			Pattern: "strategy list",
			Help:    "List indexing strategies",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  policy,
			Build:   func(*clir.Request) (any, error) { return strategyListCommand{}, nil },
		},
		{
			Pattern: "strategy add summary <name>",
			Help:    "Create a per-message summary strategy",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  policy,
			Build:   buildStrategyAddSummaryCommand,
		},
		{
			Pattern: "strategy add embedding <name>",
			Help:    "Create a per-message embedding strategy",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  policy,
			Build:   buildStrategyAddEmbeddingCommand,
		},
		{
			Pattern: "run <strategy>",
			Help:    "Run an indexing strategy for the current thread",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  policy,
			Build:   buildRunCommand,
		},
		{
			Pattern: "stats",
			Help:    "Show indexing stats",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  policy,
			Build:   func(*clir.Request) (any, error) { return statsCommand{}, nil },
		},
	}
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	for _, err := range []error{
		commandengine.RegisterPattern[strategyListCommand](registry, "strategy list", c.handleStrategyList),
		commandengine.RegisterPattern[strategyAddSummaryCommand](registry, "strategy add summary <name>", c.handleStrategyAddSummary),
		commandengine.RegisterPattern[strategyAddEmbeddingCommand](registry, "strategy add embedding <name>", c.handleStrategyAddEmbedding),
		commandengine.RegisterPattern[runCommand](registry, "run <strategy>", c.handleRun),
		commandengine.RegisterPattern[statsCommand](registry, "stats", c.handleStats),
	} {
		if err != nil {
			return err
		}
	}
	return nil
}

func buildStrategyAddSummaryCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("indexing strategy add summary", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	completion := fs.String("completion", "", "Completion component ref, for example llamacpp/local")
	model := fs.String("model", "", "Completion model name")
	prompt := fs.String("prompt", defaultSummaryPrompt, "Summary prompt")
	targetChars := fs.Int("target-chars", 0, "Target summary characters")
	batchSize := fs.Int("batch-size", 0, "Summary batch size")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected strategy arguments: %s", strings.Join(fs.Args(), " "))
	}
	cmd := strategyAddSummaryCommand{Name: strings.TrimSpace(req.Params["name"]), Completion: strings.TrimSpace(*completion), Model: strings.TrimSpace(*model), Prompt: strings.TrimSpace(*prompt), TargetChars: *targetChars, BatchSize: *batchSize}
	if cmd.Name == "" {
		return nil, fmt.Errorf("missing strategy name")
	}
	if cmd.Completion == "" {
		return nil, fmt.Errorf("missing --completion")
	}
	if cmd.Model == "" {
		return nil, fmt.Errorf("missing --model")
	}
	if cmd.TargetChars <= 0 {
		return nil, fmt.Errorf("missing --target-chars")
	}
	return cmd, nil
}

func buildStrategyAddEmbeddingCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("indexing strategy add embedding", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	embedder := fs.String("embedder", "", "Embedding component ref, for example llamacpp/local")
	model := fs.String("model", "", "Embedding model name")
	batchSize := fs.Int("batch-size", 0, "Embedding batch size")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected strategy arguments: %s", strings.Join(fs.Args(), " "))
	}
	cmd := strategyAddEmbeddingCommand{Name: strings.TrimSpace(req.Params["name"]), Embedder: strings.TrimSpace(*embedder), Model: strings.TrimSpace(*model), BatchSize: *batchSize}
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

func buildRunCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("indexing run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	maxMessages := fs.Int("max-messages", 0, "Maximum recent messages to index")
	batchSize := fs.Int("batch-size", 0, "Batch size override")
	var scope scopeFlags
	bindScopeFlags(fs, &scope)
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected run arguments: %s", strings.Join(fs.Args(), " "))
	}
	strategy := strings.TrimSpace(req.Params["strategy"])
	if strategy == "" {
		return nil, fmt.Errorf("missing strategy")
	}
	return runCommand{Strategy: strategy, MaxMessages: *maxMessages, BatchSize: *batchSize, Scope: scope}, nil
}

func bindScopeFlags(fs *flag.FlagSet, scope *scopeFlags) {
	fs.StringVar(&scope.Chat, "chat", "", "Scope to a chat id")
	fs.StringVar(&scope.Thread, "thread", "", "Scope to a thread id")
	fs.BoolVar(&scope.All, "all", false, "Use all messages")
}

func (c *Component) handleStrategyList(ctx context.Context, req commandengine.Request, cmd strategyListCommand) (commandengine.Result, error) {
	strategies, err := c.store.listStrategies(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(strategies) == 0 {
		return commandengine.Result{Text: "no indexing strategies"}, nil
	}
	var lines []string
	for _, strategy := range strategies {
		line := fmt.Sprintf("%s type=%s provider=%s model=%s enabled=%t", strategy.Name, strategy.Type, strategy.ProviderRef, strategy.Model, strategy.Enabled)
		if strategy.TargetChars > 0 {
			line += fmt.Sprintf(" target_chars=%d", strategy.TargetChars)
		}
		lines = append(lines, line)
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) handleStrategyAddSummary(ctx context.Context, req commandengine.Request, cmd strategyAddSummaryCommand) (commandengine.Result, error) {
	strategy := indexStrategy{Name: cmd.Name, Type: StrategyTypeSummary, ProviderRef: cmd.Completion, Model: cmd.Model, Prompt: cmd.Prompt, TargetChars: cmd.TargetChars, BatchSize: cmd.BatchSize}
	if err := c.store.saveStrategy(ctx, &strategy); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("indexing summary strategy saved: %s", strategy.Name)}, nil
}

func (c *Component) handleStrategyAddEmbedding(ctx context.Context, req commandengine.Request, cmd strategyAddEmbeddingCommand) (commandengine.Result, error) {
	strategy := indexStrategy{Name: cmd.Name, Type: StrategyTypeEmbedding, ProviderRef: cmd.Embedder, Model: cmd.Model, BatchSize: cmd.BatchSize}
	if err := c.store.saveStrategy(ctx, &strategy); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("indexing embedding strategy saved: %s", strategy.Name)}, nil
}

func (c *Component) handleRun(ctx context.Context, req commandengine.Request, cmd runCommand) (commandengine.Result, error) {
	scope, err := resolveScope(req.Context, cmd.Scope)
	if err != nil {
		return commandengine.Result{}, err
	}
	started := time.Now()
	result, err := c.RunStrategy(ctx, RunRequest{Strategy: cmd.Strategy, Scope: scope, MaxMessages: cmd.MaxMessages, BatchSize: cmd.BatchSize})
	if err != nil {
		return commandengine.Result{}, err
	}
	text := fmt.Sprintf("indexing run %s\ntype: %s\nprovider: %s\nscope: %s\nmessages: %d\ncreated: %d\nupdated: %d\nskipped: %d\nrun_id: %s\nelapsed: %s", result.Strategy, result.Type, result.ProviderRef, scopeText(scope), result.Messages, result.Created, result.Updated, result.Skipped, result.RunID, time.Since(started).Round(100*time.Millisecond))
	return commandengine.Result{Text: text}, nil
}

func (c *Component) handleStats(ctx context.Context, req commandengine.Request, cmd statsCommand) (commandengine.Result, error) {
	stats, err := c.store.stats(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("indexing stats\nstrategies: %d\nruns: %d\nsummaries: %d\nembeddings: %d", stats.Strategies, stats.Runs, stats.Summaries, stats.Embeddings)}, nil
}
