package heartbeat

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

const Type = "heartbeat"

// Component owns the heartbeat command surface. Recurrence is deliberately
// delegated to the scheduler repository; heartbeat owns what a tick means.
type Component struct {
	registration      coremodel.Component
	jobs              repository.ScheduledJobRepository
	chatPayloadSender component.ChatPayloadSender
	updateFeeds       []component.UpdateFeed
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)
var _ component.ChatPayloadSenderReceiver = (*Component)(nil)
var _ component.UpdateFeedReceiver = (*Component)(nil)

func New(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage, sender component.ChatPayloadSender, feeds ...component.UpdateFeed) (component.Component, error) {
	_, _, _ = ctx, runtime, home
	if storage == nil {
		return nil, fmt.Errorf("missing heartbeat storage")
	}
	return &Component{registration: registration, jobs: storage.ScheduledJobs(), chatPayloadSender: sender, updateFeeds: feeds}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) UsesLocalCommandRoutes() bool { return true }

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
