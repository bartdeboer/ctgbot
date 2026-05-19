package gmailv2

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	gmailapi "google.golang.org/api/gmail/v1"
)

const (
	Type                 = "gmailv2"
	EventMessageReceived = "email.received"
	DefaultUserID        = "me"
)

type WorkspaceResolver func(ctx context.Context, chat coremodel.Chat) (string, error)

type Options struct {
	Service               *gmailapi.Service
	OAuthClientConfigPath string
	Logger                *log.Logger
	ResolveChatWorkspace  WorkspaceResolver
}

func New(
	ctx context.Context,
	registration coremodel.Component,
	runtime runtimepkg.Factory,
	home runtimepkg.Home,
	storage repository.Storage,
	service *gmailapi.Service,
) (component.Component, error) {
	return NewWithOptions(ctx, registration, runtime, home, storage, Options{Service: service})
}

func NewWithOptions(
	ctx context.Context,
	registration coremodel.Component,
	runtime runtimepkg.Factory,
	home runtimepkg.Home,
	storage repository.Storage,
	options Options,
) (component.Component, error) {
	_, _ = ctx, runtime
	config, err := loadComponentConfig(home.Path)
	if err != nil {
		return nil, err
	}
	store, err := openStore(home.Path, config.DefaultShowFull)
	if err != nil {
		return nil, err
	}
	c := &Component{
		registration:          registration,
		componentID:           registration.ID,
		home:                  home,
		storage:               storage,
		store:                 store,
		Service:               options.Service,
		UserID:                config.UserID,
		componentConfig:       config,
		oauthClientConfigPath: strings.TrimSpace(options.OAuthClientConfigPath),
		logger:                options.Logger,
		resolveChatWorkspace:  options.ResolveChatWorkspace,
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
	home         runtimepkg.Home
	storage      repository.Storage
	store        *store

	Service *gmailapi.Service
	UserID  string

	componentConfig       ComponentConfig
	oauthClientConfigPath string
	mailboxEmail          string
	logger                *log.Logger
	clientOverride        gmailClient
	resolveChatWorkspace  WorkspaceResolver
	chatPayloadSender     component.ChatPayloadSender
}

var (
	_ component.Component                 = (*Component)(nil)
	_ component.InboundSource             = (*Component)(nil)
	_ component.ProfileOwner              = (*Component)(nil)
	_ component.Authenticator             = (*Component)(nil)
	_ component.AuthStatusReporter        = (*Component)(nil)
	_ component.SourceBindingDefaults     = (*Component)(nil)
	_ component.SkillProvider             = (*Component)(nil)
	_ component.CommandSurface            = (*Component)(nil)
	_ component.LocalCommandSurface       = (*Component)(nil)
	_ component.MessageSender             = (*Component)(nil)
	_ component.ChatPayloadSenderReceiver = (*Component)(nil)
	_ configsurface.ConfigSurface         = (*Component)(nil)
)

func (c *Component) Type() string { return Type }

func (c *Component) SetChatPayloadSender(sender component.ChatPayloadSender) {
	if c != nil {
		c.chatPayloadSender = sender
	}
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

func (c *Component) InboundEventFromStoredMessage(record storedMessage) component.InboundEvent {
	return c.inboundEventFromStoredMessage(record, "")
}

func (c *Component) inboundEventFromStoredMessage(record storedMessage, textBody string) component.InboundEvent {
	sender := strings.TrimSpace(record.FromLabel)
	if sender == "" {
		sender = strings.TrimSpace(record.FromEmail)
	}
	if sender == "" {
		sender = "Email"
	}
	return component.InboundEvent{
		ComponentID: c.componentID,
		ExternalID:  strings.TrimSpace(record.GmailMessageID),
		Payload: message.InboundPayload{
			ProviderType:      Type,
			ProviderChannelID: c.providerChannelID(),
			ProviderThreadID:  c.providerChannelID(),
			ProviderMessageID: strings.TrimSpace(record.GmailMessageID),
			ChatLabel:         c.providerChannelID(),
			Actor:             message.Actor{ID: sender, Label: sender, Roles: []simplerbac.Role{simplerbac.RoleUser}},
			Text:              message.TextMessage{Text: c.inboundPrompt(record, textBody)},
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

func senderLabel(message *gmailapi.Message) string { return headerValue(message, "From") }

func (c *Component) logf(format string, args ...any) {
	if c != nil && c.logger != nil {
		c.logger.Printf(format, args...)
	}
}
