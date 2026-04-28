// Package gmail contains the component-model Gmail implementation skeleton.
package gmail

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	gmailapi "google.golang.org/api/gmail/v1"
)

const (
	ComponentType        = "gmail"
	EventMessageReceived = "email.received"
	DefaultUserID        = "me"
)

type Component struct {
	Service *gmailapi.Service
	UserID  string
}

var _ component.Component = (*Component)(nil)
var _ component.EventSource = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)

func New(service *gmailapi.Service) *Component {
	return &Component{Service: service, UserID: DefaultUserID}
}

func (c *Component) Type() string {
	return ComponentType
}

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{{
		RelativePath: "token.json",
		Required:     true,
		Sensitive:    true,
	}}
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return nil
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	return nil
}

func (c *Component) RunEvents(ctx context.Context, emit component.InboundEventEmitter) error {
	if c == nil || c.Service == nil {
		return fmt.Errorf("missing gmail service")
	}
	if emit == nil {
		return fmt.Errorf("missing inbound event emitter")
	}
	return fmt.Errorf("gmail event polling is not implemented yet")
}

func (c *Component) GetMessage(ctx context.Context, messageID string) (*gmailapi.Message, error) {
	if c == nil || c.Service == nil {
		return nil, fmt.Errorf("missing gmail service")
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, fmt.Errorf("missing gmail message id")
	}
	return c.Service.Users.Messages.Get(c.userID(), messageID).Format("full").Context(ctx).Do()
}

func (c *Component) InboundEventFromMessage(message *gmailapi.Message) component.InboundEvent {
	if message == nil {
		return component.InboundEvent{SourceType: ComponentType, EventType: EventMessageReceived}
	}
	return component.InboundEvent{
		SourceType: ComponentType,
		EventType:  EventMessageReceived,
		ExternalID: strings.TrimSpace(message.Id),
		Text:       strings.TrimSpace(message.Snippet),
		Metadata: map[string]string{
			"gmail.message_id": strings.TrimSpace(message.Id),
			"gmail.thread_id":  strings.TrimSpace(message.ThreadId),
		},
	}
}

func (c *Component) userID() string {
	if c == nil || strings.TrimSpace(c.UserID) == "" {
		return DefaultUserID
	}
	return strings.TrimSpace(c.UserID)
}
