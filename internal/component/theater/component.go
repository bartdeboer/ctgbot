package theater

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	messagingdomain "github.com/bartdeboer/ctgbot/internal/messaging"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

const Type = "theater"

type Component struct {
	registration coremodel.Component
	store        *store
	storage      repository.Storage
	messages     *messagingdomain.Service
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ component.CommandDescriptionSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)
var _ component.UpdateFeed = (*Component)(nil)

func New(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
	_, _ = ctx, runtime
	store, err := openStore(home.Path)
	if err != nil {
		return nil, err
	}
	return &Component{registration: registration, store: store, storage: storage, messages: messagingdomain.New(storage)}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) CommandDescriptions() []commandengine.Description {
	return []commandengine.Description{{
		Pattern: "",
		Help:    "thread subscriptions and shared message boards",
		Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
		Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
	}}
}

func (c *Component) NewUpdates(ctx context.Context, req component.UpdateRequest) ([]component.UpdateNotice, error) {
	if c == nil || c.store == nil {
		return nil, fmt.Errorf("missing theater store")
	}
	return c.pendingUpdates(ctx, req.ThreadID)
}
