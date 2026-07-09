package outlook

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
)

const (
	Type                 = "outlook"
	EventMessageReceived = "email.received"
)

type WorkspaceResolver func(ctx context.Context, chat coremodel.Chat) (string, error)

type Options struct {
	Logger               *log.Logger
	ResolveChatWorkspace WorkspaceResolver
}

type Component struct {
	registration         coremodel.Component
	componentID          modeluuid.UUID
	profile              runtimepkg.Profile
	storage              repository.Storage
	store                *store
	componentConfig      ComponentConfig
	mailboxEmail         string
	logger               *log.Logger
	resolveChatWorkspace WorkspaceResolver
	clientOverride       outlookClient
}

func New(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, profile runtimepkg.Profile, storage repository.Storage) (component.Component, error) {
	return NewWithOptions(ctx, registration, runtime, profile, storage, Options{})
}

func NewWithOptions(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, profile runtimepkg.Profile, storage repository.Storage, options Options) (component.Component, error) {
	_, _ = ctx, runtime
	config, err := loadComponentConfig(profile.Path)
	if err != nil {
		return nil, err
	}
	store, err := openStore(profile.Path)
	if err != nil {
		return nil, err
	}
	c := &Component{registration: registration, componentID: registration.ID, profile: profile, storage: storage, store: store, componentConfig: config, logger: options.Logger, resolveChatWorkspace: options.ResolveChatWorkspace}
	if state, err := c.loadState(); err == nil {
		c.mailboxEmail = strings.TrimSpace(state.MailboxEmail)
	}
	return c, nil
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
	_ configsurface.ConfigSurface     = (*Component)(nil)
)

func (c *Component) Type() string { return Type }

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{{RelativePath: OAuthClientFilename, Required: false, Sensitive: true}, {RelativePath: TokenFilename, Required: true, Sensitive: true}, {RelativePath: ComponentConfigFilename, Required: false}, {RelativePath: StateFilename, Required: false}}
}

func (c *Component) userID() string {
	if c == nil || strings.TrimSpace(c.componentConfig.UserID) == "" {
		return DefaultUserID
	}
	return strings.TrimSpace(c.componentConfig.UserID)
}
func (c *Component) providerChannelID() string {
	if c == nil {
		return DefaultUserID
	}
	if v := strings.TrimSpace(c.mailboxEmail); v != "" {
		return v
	}
	if v := strings.TrimSpace(c.componentConfig.MailboxEmail); v != "" {
		return v
	}
	return c.userID()
}
func (c *Component) logf(format string, args ...any) {
	if c != nil && c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

func (c *Component) inboundEventFromStoredMessage(record storedMessage) component.InboundEvent {
	sender := strings.TrimSpace(record.FromLabel)
	if sender == "" {
		sender = strings.TrimSpace(record.FromEmail)
	}
	if sender == "" {
		sender = "Outlook"
	}
	return component.InboundEvent{ComponentID: c.componentID, ExternalID: record.OutlookMessageID, Payload: message.InboundPayload{ProviderType: Type, ProviderChannelID: c.providerChannelID(), ProviderThreadID: c.providerChannelID(), ProviderMessageID: record.OutlookMessageID, ChatLabel: c.providerChannelID(), Actor: message.Actor{ID: sender, Label: sender, Roles: []simplerbac.Role{simplerbac.RoleUser}}, Text: message.TextMessage{Text: c.inboundPrompt(record)}}}
}

func (c *Component) DefaultSourceExternalChannelID(ctx context.Context) (string, error) {
	if v := strings.TrimSpace(c.providerChannelID()); v != "" && v != DefaultUserID {
		return v, nil
	}
	client, err := c.client(ctx)
	if err != nil {
		return "", err
	}
	me, err := client.Me(ctx)
	if err != nil {
		return "", fmt.Errorf("outlook source binding needs auth or component.json mailbox_email: %w", err)
	}
	c.mailboxEmail = strings.TrimSpace(firstNonEmpty(me.Mail, me.UserPrincipalName))
	if c.mailboxEmail == "" {
		return "", fmt.Errorf("outlook source binding needs auth or component.json mailbox_email")
	}
	state, _ := c.loadState()
	state.MailboxEmail = c.mailboxEmail
	_ = c.saveState(state)
	return c.mailboxEmail, nil
}
