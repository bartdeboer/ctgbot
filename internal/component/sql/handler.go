package sqlcomponent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

func (c *Component) handleSQL(ctx context.Context, req commandengine.Request, cmd Command) (commandengine.Result, error) {
	if req.Context.ChatID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("sql access denied: missing chat context")
	}
	if c == nil || c.Config == nil {
		return commandengine.Result{}, fmt.Errorf("sql access denied: missing config")
	}
	if !c.Config.Chat(req.Context.ChatID).AgentDBAccessEnabled() {
		return commandengine.Result{}, fmt.Errorf("sql access denied: enable with `ctgbot config chat %s set chat.enable-agent-db-access true`", req.Context.ChatID)
	}
	if c.Executor == nil {
		return commandengine.Result{}, fmt.Errorf("missing sql executor")
	}

	query := strings.TrimSpace(cmd.Query)
	if query == "" {
		return commandengine.Result{}, fmt.Errorf("missing SQL query")
	}
	if err := validateSingleStatement(query); err != nil {
		return commandengine.Result{}, err
	}
	if cmd.Write {
		return c.execWrite(ctx, query)
	}
	if !isReadStatement(query) {
		return commandengine.Result{}, fmt.Errorf("mutating SQL requires --write; read mode only allows SELECT or WITH SELECT")
	}
	limit := cmd.Limit
	if limit <= 0 {
		limit = defaultRowLimit
	}
	if limit > maxRowLimit {
		limit = maxRowLimit
	}
	return c.execRead(ctx, query, limit)
}

func (c *Component) execWrite(ctx context.Context, query string) (commandengine.Result, error) {
	result, err := c.Executor.ExecContext(ctx, query)
	if err != nil {
		return commandengine.Result{}, err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return commandengine.Result{Text: "sql write completed"}, nil
	}
	return commandengine.Result{Text: fmt.Sprintf("sql write completed: rows_affected=%d", rowsAffected)}, nil
}

func (c *Component) execRead(ctx context.Context, query string, limit int) (commandengine.Result, error) {
	rows, err := c.Executor.QueryContext(ctx, query)
	if err != nil {
		return commandengine.Result{}, err
	}
	defer rows.Close()

	text, err := renderRows(rows, limit)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: text}, rows.Err()
}
