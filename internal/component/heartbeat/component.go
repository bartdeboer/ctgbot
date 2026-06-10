package heartbeat

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

const Type = "heartbeat"

// Component owns the heartbeat command surface. Recurrence is persisted as a
// timed intent; heartbeat owns the user-facing command surface and update text.
type Component struct {
	registration      coremodel.Component
	intents           repository.TimedIntentRepository
	chatPayloadSender component.ChatPayloadSender
	updateFeeds       []component.UpdateFeed
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ component.CommandDescriptionSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)
var _ component.ChatPayloadSenderReceiver = (*Component)(nil)
var _ component.UpdateFeedReceiver = (*Component)(nil)

func New(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage, sender component.ChatPayloadSender, feeds ...component.UpdateFeed) (component.Component, error) {
	_, _, _ = ctx, runtime, home
	if storage == nil {
		return nil, fmt.Errorf("missing heartbeat storage")
	}
	return &Component{registration: registration, intents: storage.TimedIntents(), chatPayloadSender: sender, updateFeeds: feeds}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) CommandDescriptions() []commandengine.Description {
	policy := simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent)
	return []commandengine.Description{{
		Pattern: "",
		Help:    "autonomous keepalive and self-scheduling",
		Sources: []commandengine.Source{commandengine.SourceMessage, commandengine.SourceHostbridge},
		Policy:  policy,
	}}
}

func (c *Component) SetChatPayloadSender(sender component.ChatPayloadSender) {
	if c != nil {
		c.chatPayloadSender = sender
	}
}

func (c *Component) SetUpdateFeeds(feeds []component.UpdateFeed) {
	if c != nil {
		c.updateFeeds = append([]component.UpdateFeed(nil), feeds...)
	}
}
