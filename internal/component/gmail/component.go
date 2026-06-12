package gmail

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	gmailapi "google.golang.org/api/gmail/v1"
)

const (
	Type                 = "gmail"
	EventMessageReceived = "email.received"
	DefaultUserID        = "me"
)

type Options struct {
	Service               *gmailapi.Service
	OAuthClientConfigPath string
	Logger                *log.Logger
}

func New(
	ctx context.Context,
	registration coremodel.Component,
	runtime runtimepkg.Factory,
	profile runtimepkg.Profile,
	storage repository.Storage,
	service *gmailapi.Service,
) (component.Component, error) {
	return NewWithOptions(ctx, registration, runtime, profile, storage, Options{Service: service})
}

func NewWithOptions(
	ctx context.Context,
	registration coremodel.Component,
	runtime runtimepkg.Factory,
	profile runtimepkg.Profile,
	storage repository.Storage,
	options Options,
) (component.Component, error) {
	_, _, _ = ctx, runtime, storage
	config, err := loadComponentConfig(profile.Path)
	if err != nil {
		return nil, err
	}
	c := &Component{
		registration:          registration,
		componentID:           registration.ID,
		profile:               profile,
		storage:               storage,
		Service:               options.Service,
		UserID:                config.UserID,
		componentConfig:       config,
		oauthClientConfigPath: strings.TrimSpace(options.OAuthClientConfigPath),
		logger:                options.Logger,
	}
	if strings.TrimSpace(c.UserID) == "" {
		c.UserID = DefaultUserID
	}
	if c.Service == nil {
		service, err := c.serviceFromStoredToken(ctx)
		if err != nil && !isMissingAuthMaterial(err) {
			return nil, err
		}
		c.Service = service
	}
	if state, err := c.loadState(); err == nil {
		c.mailboxEmail = strings.TrimSpace(state.MailboxEmail)
	}
	return c, nil
}

type Component struct {
	registration coremodel.Component
	componentID  modeluuid.UUID
	profile      runtimepkg.Profile
	storage      repository.Storage

	Service *gmailapi.Service
	UserID  string

	componentConfig       ComponentConfig
	oauthClientConfigPath string
	mailboxEmail          string
	logger                *log.Logger
	clientOverride        gmailClient
}

var (
	_ component.Component             = (*Component)(nil)
	_ component.InboundSource         = (*Component)(nil)
	_ component.ProfileOwner          = (*Component)(nil)
	_ component.Authenticator         = (*Component)(nil)
	_ component.AuthStatusReporter    = (*Component)(nil)
	_ component.SourceBindingDefaults = (*Component)(nil)
	_ component.SkillProvider         = (*Component)(nil)
	_ component.CommandSurface        = (*Component)(nil)
	_ component.LocalCommandSurface   = (*Component)(nil)
	_ component.MessageSender         = (*Component)(nil)
)

func (c *Component) Type() string {
	return Type
}

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: OAuthClientFilename, Required: false, Sensitive: true},
		{RelativePath: TokenFilename, Required: true, Sensitive: true},
		{RelativePath: ComponentConfigFilename, Required: false, Sensitive: false},
		{RelativePath: StateFilename, Required: false, Sensitive: false},
	}
}

func (c *Component) GetMessage(ctx context.Context, messageID string) (*gmailapi.Message, error) {
	client, err := c.client(ctx)
	if err != nil {
		return nil, err
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return nil, fmt.Errorf("missing gmail message id")
	}
	return client.GetMessage(ctx, c.userID(), messageID)
}

func (c *Component) InboundEventFromMessage(gmailMessage *gmailapi.Message) component.InboundEvent {
	if gmailMessage == nil {
		return component.InboundEvent{
			ComponentID: c.componentID,
			Payload: message.InboundPayload{
				ProviderType:      Type,
				ProviderChannelID: c.providerChannelID(),
				ChatLabel:         c.providerChannelID(),
				Actor: message.Actor{
					ID:    "email",
					Label: "Email",
					Roles: []simplerbac.Role{simplerbac.RoleUser},
				},
			},
		}
	}

	sender := senderLabel(gmailMessage)
	if sender == "" {
		sender = "Email"
	}

	return component.InboundEvent{
		ComponentID: c.componentID,
		ExternalID:  strings.TrimSpace(gmailMessage.Id),
		Payload: message.InboundPayload{
			ProviderType:      Type,
			ProviderChannelID: c.providerChannelID(),
			ProviderThreadID:  c.providerChannelID(),
			ProviderMessageID: strings.TrimSpace(gmailMessage.Id),
			ChatLabel:         c.providerChannelID(),
			Actor: message.Actor{
				ID:    sender,
				Label: sender,
				Roles: []simplerbac.Role{simplerbac.RoleUser},
			},
			Text: message.TextMessage{
				Text: emailPromptTextForComponent(c.registration.Ref(), gmailMessage),
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

func (c *Component) providerChannelID() string {
	if c == nil {
		return DefaultUserID
	}
	if value, ok := c.localProviderChannelID(); ok {
		return value
	}
	return c.userID()
}

func (c *Component) localProviderChannelID() (string, bool) {
	if c == nil {
		return "", false
	}
	if value := strings.TrimSpace(c.mailboxEmail); value != "" {
		return value, true
	}
	if value := strings.TrimSpace(c.componentConfig.MailboxEmail); value != "" {
		return value, true
	}
	if value := c.userID(); !strings.EqualFold(value, DefaultUserID) {
		return value, true
	}
	return "", false
}

func senderLabel(message *gmailapi.Message) string {
	return headerValue(message, "From")
}

func (c *Component) logf(format string, args ...any) {
	if c != nil && c.logger != nil {
		c.logger.Printf(format, args...)
	}
}
