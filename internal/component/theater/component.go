package theater

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

const (
	Type                        = "theater"
	defaultTheaterWorkspaceRoot = "/workspace/theaters"
)

type Component struct {
	registration coremodel.Component
	store        *store
	workspaceRoot string
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ component.CommandDescriptionSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)
var _ component.UpdateFeed = (*Component)(nil)

func New(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
	_, _, _ = ctx, runtime, storage
	store, err := openStore(home.Path)
	if err != nil {
		return nil, err
	}
	return &Component{registration: registration, store: store, workspaceRoot: defaultTheaterWorkspaceRoot}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) workspacePath(name string) string {
	root := defaultTheaterWorkspaceRoot
	if c != nil && strings.TrimSpace(c.workspaceRoot) != "" {
		root = strings.TrimSpace(c.workspaceRoot)
	}
	return filepath.Join(root, normalizeName(name))
}

func (c *Component) CommandDescriptions() []commandengine.Description {
	return []commandengine.Description{{
		Pattern: "",
		Help:    "publish/subscribe message channels between agents",
		Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
		Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
	}}
}

func (c *Component) NewUpdates(ctx context.Context, req component.UpdateRequest) ([]component.UpdateNotice, error) {
	if c == nil || c.store == nil {
		return nil, fmt.Errorf("missing theater store")
	}
	return c.store.pendingUpdates(ctx, req.ThreadID)
}
