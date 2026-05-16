package semantic

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type searchCommand struct {
	Query     string
	Limit     int
	BatchSize int
	MinScore  float64
}

func RegisterGobTypes(register func(any)) {
	register(searchCommand{})
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		{
			Pattern: "search <query>",
			Help:    "Search current thread history semantically",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
			Build:   buildSearchCommand,
		},
	}
}

func buildSearchCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("semantic search", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	limit := fs.Int("limit", 0, "Maximum results")
	batchSize := fs.Int("batch-size", 0, "Messages per scoring batch")
	minScore := fs.Float64("min-score", 0, "Minimum score to include")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected search arguments: %s", strings.Join(fs.Args(), " "))
	}
	query := strings.TrimSpace(req.Params["query"])
	if query == "" {
		return nil, fmt.Errorf("missing search query")
	}
	return searchCommand{Query: query, Limit: *limit, BatchSize: *batchSize, MinScore: *minScore}, nil
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	return commandengine.RegisterPattern[searchCommand](registry, "search <query>", c.handleSearch)
}

func (c *Component) handleSearch(ctx context.Context, req commandengine.Request, cmd searchCommand) (commandengine.Result, error) {
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
	response, err := c.Search(ctx, component.SearchRequest{Query: cmd.Query, ChatID: req.Context.ChatID, ThreadID: threadID, Limit: limit, BatchSize: cmd.BatchSize, MinScore: cmd.MinScore})
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: c.renderSearchResponse(cmd.Query, response)}, nil
}

func (c *Component) renderSearchResponse(query string, response component.SearchResponse) string {
	if len(response.Results) == 0 {
		return "semantic search: " + query + "\n(no results)"
	}
	var lines []string
	lines = append(lines, "semantic search: "+query)
	for i, result := range response.Results {
		lines = append(lines, "")
		lines = append(lines, fmt.Sprintf("%d. message_id=%s thread_id=%s score=%.2f", i+1, result.MessageID, result.ThreadID, result.Score))
		if strings.TrimSpace(result.Excerpt) != "" {
			lines = append(lines, "excerpt: "+result.Excerpt)
		}
		if strings.TrimSpace(result.Reason) != "" {
			lines = append(lines, "reason: "+result.Reason)
		}
	}
	return strings.Join(lines, "\n")
}
