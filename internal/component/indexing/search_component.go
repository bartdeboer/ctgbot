package indexing

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

const (
	SearchType                     = "search"
	DefaultIndexingComponentRef    = "indexing"
	DefaultSearchEmbeddingStrategy = "default-search-embedding"
	DefaultSearchTitleStrategy     = "default-search-title"
	DefaultSearchLimit             = 10
)

const (
	searchModeEmbedding = "embedding"
	searchModeKeyword   = "keyword"
)

type SearchComponent struct {
	resolver ComponentResolver
	messages component.SearchMessageSource
}

var _ component.Component = (*SearchComponent)(nil)
var _ component.CommandSurface = (*SearchComponent)(nil)
var _ component.LocalCommandSurface = (*SearchComponent)(nil)
var _ component.SearchMessageSourceReceiver = (*SearchComponent)(nil)

func NewSearch(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, profile runtimepkg.Profile, storage repository.Storage, resolver ComponentResolver, logf func(format string, args ...any)) (component.Component, error) {
	_, _, _, _, _ = ctx, registration, runtime, profile, storage
	return &SearchComponent{resolver: resolver}, nil
}

func (c *SearchComponent) Type() string { return SearchType }

func (c *SearchComponent) SetSearchMessageSource(source component.SearchMessageSource) {
	if c != nil {
		c.messages = source
	}
}

func (c *SearchComponent) UsesLocalCommandRoutes() bool { return true }

type searchCommand struct {
	Query     string
	Mode      string
	Indexing  string
	Embedding string
	Title     string
	Limit     int
	Scope     scopeFlags
}

func (c *SearchComponent) CommandDefinitions() []commandengine.Definition {
	policy := simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent)
	return []commandengine.Definition{{
		Pattern:               "<query>",
		Help:                  "Search indexed chat messages",
		Sources:               []commandengine.Source{commandengine.SourceHostbridge, commandengine.SourceMessage},
		Policy:                policy,
		InstructionVisibility: commandengine.InstructionImportant,
		Build:                 buildIndexedSearchCommand,
	}}
}

func (c *SearchComponent) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	return commandengine.RegisterPattern[searchCommand](registry, "<query>", c.handleSearch)
}

func buildIndexedSearchCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("search", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	mode := fs.String("mode", searchModeEmbedding, "Search mode: embedding or keyword")
	indexingRef := fs.String("indexing", DefaultIndexingComponentRef, "Indexing component ref that owns the search indexes")
	embedding := fs.String("embedding", DefaultSearchEmbeddingStrategy, "Embedding strategy used for ranking")
	title := fs.String("title", DefaultSearchTitleStrategy, "Summary strategy used for result titles")
	limit := fs.Int("limit", DefaultSearchLimit, "Maximum results")
	var scope scopeFlags
	bindScopeFlags(fs, &scope)
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected search arguments: %s", strings.Join(fs.Args(), " "))
	}
	cmd := searchCommand{
		Query:     strings.TrimSpace(req.Params["query"]),
		Mode:      normalizeSearchMode(*mode),
		Indexing:  strings.TrimSpace(*indexingRef),
		Embedding: strings.TrimSpace(*embedding),
		Title:     strings.TrimSpace(*title),
		Limit:     *limit,
		Scope:     scope,
	}
	if cmd.Query == "" {
		return nil, fmt.Errorf("missing search query")
	}
	if cmd.Mode != searchModeEmbedding && cmd.Mode != searchModeKeyword {
		return nil, fmt.Errorf("unsupported search mode: %s", cmd.Mode)
	}
	if cmd.Indexing == "" {
		return nil, fmt.Errorf("missing --indexing")
	}
	if cmd.Mode == searchModeEmbedding && cmd.Embedding == "" {
		return nil, fmt.Errorf("missing --embedding")
	}
	if cmd.Limit <= 0 {
		cmd.Limit = DefaultSearchLimit
	}
	return cmd, nil
}

func normalizeSearchMode(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func (c *SearchComponent) handleSearch(ctx context.Context, req commandengine.Request, cmd searchCommand) (commandengine.Result, error) {
	scope, err := resolveScope(req.Context, cmd.Scope)
	if err != nil {
		return commandengine.Result{}, err
	}
	results, err := c.Search(ctx, SearchRequest{
		Query:             cmd.Query,
		Mode:              cmd.Mode,
		IndexingComponent: cmd.Indexing,
		EmbeddingStrategy: cmd.Embedding,
		TitleStrategy:     cmd.Title,
		Scope:             scope,
		Limit:             cmd.Limit,
	})
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: formatSearchResults(cmd.Query, results)}, nil
}

func formatSearchResults(query string, results []SearchResult) string {
	if len(results) == 0 {
		return fmt.Sprintf("search results for %q\nno matches", strings.TrimSpace(query))
	}
	var lines []string
	lines = append(lines, fmt.Sprintf("search results for %q", strings.TrimSpace(query)))
	for i, result := range results {
		label := strings.TrimSpace(result.Title)
		if label == "" {
			label = strings.TrimSpace(result.Excerpt)
		}
		if label == "" {
			label = "(empty)"
		}
		lines = append(lines, fmt.Sprintf("%d. %.3f %s %s", i+1, result.Score, result.Role, result.CreatedAt.Format("2006-01-02")))
		lines = append(lines, "   "+label)
		lines = append(lines, fmt.Sprintf("   message_id=%s thread_id=%s", result.MessageID, result.ThreadID))
	}
	return strings.Join(lines, "\n")
}
