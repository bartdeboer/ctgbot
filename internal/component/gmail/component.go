package gmail

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	gmailapi "google.golang.org/api/gmail/v1"
)

const (
	Type                 = "gmail"
	EventMessageReceived = "email.received"
	DefaultUserID        = "me"
)

func New(
	ctx context.Context,
	registration coremodel.Component,
	runtime v5runtime.Factory,
	home v5runtime.Home,
	storage repository.Storage,
	service *gmailapi.Service,
) (component.Component, error) {
	_, _, _, _, _ = ctx, runtime, home, storage, registration
	return &Component{
		componentID: registration.ID,
		Service:     service,
		UserID:      DefaultUserID,
	}, nil
}

type Component struct {
	componentID modeluuid.UUID
	Service     *gmailapi.Service
	UserID      string
}

var _ component.Component = (*Component)(nil)
var _ component.InboundSource = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)

func (c *Component) Type() string {
	return Type
}

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{{
		RelativePath: "token.json",
		Required:     true,
		Sensitive:    true,
	}}
}

func (c *Component) RunInbound(ctx context.Context, emit component.InboundEmitter) error {
	if c == nil || c.Service == nil {
		return fmt.Errorf("missing gmail service")
	}
	if emit == nil {
		return fmt.Errorf("missing inbound emitter")
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
		return component.InboundEvent{
			ComponentID: c.componentID,
			Payload: messenger.InboundPayload{
				ProviderType:   Type,
				ProviderChatID: c.userID(),
				Actor: messenger.Actor{
					ID:    "email",
					Label: "Email",
					Roles: []simplerbac.Role{simplerbac.RoleUser},
				},
			},
		}
	}

	sender := senderLabel(message)
	if sender == "" {
		sender = "Email"
	}

	return component.InboundEvent{
		ComponentID: c.componentID,
		ExternalID:  strings.TrimSpace(message.Id),
		Payload: messenger.InboundPayload{
			ProviderType:      Type,
			ProviderChatID:    c.userID(),
			ProviderThreadID:  strings.TrimSpace(message.ThreadId),
			ProviderMessageID: strings.TrimSpace(message.Id),
			Actor: messenger.Actor{
				ID:    sender,
				Label: sender,
				Roles: []simplerbac.Role{simplerbac.RoleUser},
			},
			Text: messenger.TextMessage{
				Text: strings.TrimSpace(message.Snippet),
			},
		},
	}
}

func (c *Component) userID() string {
	if c == nil || strings.TrimSpace(c.UserID) == "" {
		return DefaultUserID
	}
	return strings.TrimSpace(c.UserID)
}

func senderLabel(message *gmailapi.Message) string {
	if message == nil || message.Payload == nil {
		return ""
	}
	for _, header := range message.Payload.Headers {
		if strings.EqualFold(strings.TrimSpace(header.Name), "From") {
			return strings.TrimSpace(header.Value)
		}
	}
	return ""
}
