package sqlcomponent

import (
	"context"
	databasesql "database/sql"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
	"gorm.io/gorm"
)

const Type = "sql"

const (
	defaultRowLimit = 100
	maxRowLimit     = 500
)

type Executor interface {
	QueryContext(ctx context.Context, query string, args ...any) (*databasesql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (databasesql.Result, error)
}

type Component struct {
	Executor Executor
	Config   *appstate.Config
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)

type Command struct {
	Query string
	Write bool
	Limit int
}

func New(db *gorm.DB, cfg *appstate.Config) (*Component, error) {
	if db == nil {
		return &Component{Config: cfg}, nil
	}
	raw, err := db.DB()
	if err != nil {
		return nil, err
	}
	return NewWithExecutor(raw, cfg), nil
}

func NewWithExecutor(executor Executor, cfg *appstate.Config) *Component {
	return &Component{Executor: executor, Config: cfg}
}

func (c *Component) Type() string { return Type }

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{sqlCommandDefinition()}
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	return commandengine.RegisterPattern[Command](registry, "sql", c.handleSQL)
}

func RegisterGobTypes(register func(any)) {
	register(Command{})
}

func sqlCommandDefinition() commandengine.Definition {
	return commandengine.Definition{
		Pattern:               "sql",
		Help:                  "Run an explicitly enabled ctgbot database SQL query",
		Build:                 buildSQLCommand,
		Sources:               []commandengine.Source{commandengine.SourceHostbridge},
		Policy:                simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		InstructionVisibility: commandengine.InstructionImportant,
	}
}

func buildSQLCommand(req *clir.Request) (any, error) {
	return buildCommandFromArgs(req.Extra)
}
