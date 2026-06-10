package app

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	brokercomponent "github.com/bartdeboer/ctgbot/internal/component/broker"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/identity"
	"github.com/bartdeboer/ctgbot/internal/inbound"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
	workspacepkg "github.com/bartdeboer/ctgbot/internal/workspace"
)

type ComponentResolver interface {
	ResolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error)
	ResolveComponent(ctx context.Context, id modeluuid.UUID) (*component.Loaded, error)
}

type WorkspaceValidator interface {
	ValidateWorkspace(name string) error
}

type ComponentManager interface {
	EnsureComponent(ctx context.Context, ref string, runtimeKind string, homePath string) (*coremodel.Component, error)
	Runtime(kind string) (runtimepkg.Factory, error)
}

type ChatRuntimeResolver interface {
	ResolveChatWorkspace(ctx context.Context, chat coremodel.Chat) (string, error)
	ResolveChatHostbridgeAllowedCommands(ctx context.Context, chat coremodel.Chat) (map[string]hostbridgeserver.AllowedCommand, error)
}

type Service interface {
	ChatAdminService
	WorkspaceAdminService
	ComponentAdminService
	BrokerService
	RuntimeImageService
	CLICommandSurfaces(ctx context.Context) ([]component.CommandSurface, error)
	ScheduledCommandEngine(ctx context.Context) (*commandengine.Engine, error)
	UpdateFeeds(ctx context.Context) ([]component.UpdateFeed, error)
	ControllerCommandEngine(ctx context.Context) (*commandengine.Engine, error)
	InstanceIdentity(ctx context.Context) (identity.Identity, error)
	ScheduledJobRepository() repository.ScheduledJobRepository
	TimedIntentRepository() repository.TimedIntentRepository
}

type ChatAdminService interface {
	CreateChat(ctx context.Context, label string, workspace string) (coremodel.Chat, error)
	ListChats(ctx context.Context) ([]ChatInfo, error)
	ResolveChatRef(ctx context.Context, ref string) (modeluuid.UUID, error)
	ListInboundDrops(ctx context.Context) ([]InboundDropInfo, error)
	SetChatWorkspace(ctx context.Context, chatID modeluuid.UUID, workspace string) (coremodel.Chat, error)
	BindInboundChat(ctx context.Context, componentRef string, externalChannelID string, label string, roleFlag string) (ChatBindResult, error)
	AddChatComponent(ctx context.Context, chatID modeluuid.UUID, role coremodel.ChatComponentRole, componentRef string, externalChannelID string) (ChatComponentAddResult, error)
	RemoveChatComponent(ctx context.Context, chatID modeluuid.UUID, role coremodel.ChatComponentRole, componentRef string) (ChatComponentRemoveResult, error)
	ListChatComponents(ctx context.Context, chatID modeluuid.UUID) ([]ChatComponentInfo, error)
	AddChatComponentFilter(ctx context.Context, chatRef string, sourceRef string, externalChannelID string, filterRef string) (ComponentFilterAddResult, error)
	RemoveChatComponentFilter(ctx context.Context, chatRef string, sourceRef string, externalChannelID string, filterRef string) (ComponentFilterRemoveResult, error)
	ClearChatComponentFilters(ctx context.Context, chatRef string, sourceRef string, externalChannelID string) (ComponentFilterClearResult, error)
	ListChatComponentFilters(ctx context.Context, chatRef string, sourceRef string, externalChannelID string) (ComponentFilterListResult, error)
}

type ComponentAdminService interface {
	ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*component.Loaded, error)
	ResolveChatWorkspace(ctx context.Context, chat coremodel.Chat) (string, error)
	ResolveChatHostbridgeAllowedCommands(ctx context.Context, chat coremodel.Chat) (map[string]hostbridgeserver.AllowedCommand, error)
	RegisterComponent(ctx context.Context, componentRef string, runtimeKind string, homePath string) (RegisterComponentResult, error)
	UnregisterComponent(ctx context.Context, componentRef string) (UnregisterComponentResult, error)
	ListComponents(ctx context.Context) ([]ComponentInfo, error)
	RunComponentCommand(ctx context.Context, request ComponentCommandRequest) (ComponentCommandResult, error)
}

type BrokerService interface {
	ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*component.Loaded, error)
	ResolveChatWorkspace(ctx context.Context, chat coremodel.Chat) (string, error)
	ResolveChatHostbridgeAllowedCommands(ctx context.Context, chat coremodel.Chat) (map[string]hostbridgeserver.AllowedCommand, error)
	AdmitInbound(ctx context.Context, event component.InboundEvent) (inbound.Admission, error)
	Chat(ctx context.Context, chatID modeluuid.UUID) (*coremodel.Chat, error)
	Thread(ctx context.Context, threadID modeluuid.UUID) (*coremodel.Thread, error)
	ThreadMessages(ctx context.Context, threadID modeluuid.UUID) ([]coremodel.ThreadMessage, error)
	ForEachMessage(ctx context.Context, scope component.MessageScope, visit component.MessageVisitor) error
	EnabledChatComponents(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ChatComponent, error)
	EnabledInboundSources(ctx context.Context) ([]component.InboundSource, error)
	CommandSurfaces(ctx context.Context, chat coremodel.Chat, inbound component.ResolvedInboundQueuer, actions brokercomponent.Actions) ([]component.CommandSurface, error)
	EnsureThread(ctx context.Context, binding coremodel.ChatComponent, componentThreadID string) (*coremodel.Thread, error)
	ComponentThreadID(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID) (string, bool, error)
	BindComponentThreadID(ctx context.Context, threadID modeluuid.UUID, componentID modeluuid.UUID, componentThreadID string) error
	RelayTarget(ctx context.Context, threadID modeluuid.UUID, binding coremodel.ChatComponent) (*message.ChatTarget, bool, error)
	StoreInboundMessage(ctx context.Context, inbound component.ResolvedInbound) (*coremodel.ThreadMessage, error)
	StoreOutboundMessage(ctx context.Context, message *coremodel.ThreadMessage, attachments []message.Media) error
	DropEvent(ctx context.Context, rejection *inbound.Rejection) (*coremodel.DroppedEvent, error)
	DropNoticeID(ctx context.Context, drop *coremodel.DroppedEvent) string
	ResolveDroppedEventID(ctx context.Context, ref string) (modeluuid.UUID, error)
	DroppedEvent(ctx context.Context, id modeluuid.UUID) (*coremodel.DroppedEvent, error)
	ListDroppedEvents(ctx context.Context, limit int) ([]coremodel.DroppedEvent, error)
	SaveDroppedEvent(ctx context.Context, drop *coremodel.DroppedEvent) error
}

type RuntimeImageService interface {
	RuntimeImageTargets(ctx context.Context) ([]runtimeimage.Target, error)
}

type service struct {
	Storage             repository.Storage
	Resolver            ComponentResolver
	ComponentManager    ComponentManager
	ChatRuntimeResolver ChatRuntimeResolver
	WorkspaceValidator  WorkspaceValidator
	Workspaces          workspacepkg.Manager
	Logf                func(format string, args ...any)
}

func NewService(storage repository.Storage, resolver ComponentResolver) Service {
	return NewServiceWithLogger(storage, resolver, nil)
}

func NewServiceWithLogger(storage repository.Storage, resolver ComponentResolver, logf func(format string, args ...any)) Service {
	service := &service{Storage: storage, Resolver: resolver, Logf: logf}
	if manager, ok := resolver.(ComponentManager); ok {
		service.ComponentManager = manager
	}
	if runtimeResolver, ok := resolver.(ChatRuntimeResolver); ok {
		service.ChatRuntimeResolver = runtimeResolver
	}
	if validator, ok := resolver.(WorkspaceValidator); ok {
		service.WorkspaceValidator = validator
	}
	if provider, ok := resolver.(WorkspaceManagerProvider); ok {
		service.Workspaces = provider.WorkspaceManager()
	}
	return service
}

func (s *service) Repository() repository.Storage {
	if s == nil {
		return nil
	}
	return s.Storage
}

func (s *service) ScheduledJobRepository() repository.ScheduledJobRepository {
	if s == nil || s.Storage == nil {
		return nil
	}
	return s.Storage.ScheduledJobs()
}

func (s *service) TimedIntentRepository() repository.TimedIntentRepository {
	if s == nil || s.Storage == nil {
		return nil
	}
	return s.Storage.TimedIntents()
}

func (s *service) ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*component.Loaded, error) {
	return s.resolveLoadedComponent(ctx, componentID)
}

func (s *service) ResolveChatWorkspace(ctx context.Context, chat coremodel.Chat) (string, error) {
	if s == nil || s.ChatRuntimeResolver == nil {
		return "", fmt.Errorf("missing chat runtime resolver")
	}
	return s.ChatRuntimeResolver.ResolveChatWorkspace(ctx, chat)
}

func (s *service) ResolveChatHostbridgeAllowedCommands(ctx context.Context, chat coremodel.Chat) (map[string]hostbridgeserver.AllowedCommand, error) {
	if s == nil || s.ChatRuntimeResolver == nil {
		return nil, fmt.Errorf("missing chat runtime resolver")
	}
	return s.ChatRuntimeResolver.ResolveChatHostbridgeAllowedCommands(ctx, chat)
}

func (s *service) componentManager() (ComponentManager, error) {
	if s == nil || s.ComponentManager == nil {
		return nil, fmt.Errorf("missing component manager")
	}
	return s.ComponentManager, nil
}

func (s *service) resolveComponentRegistration(ctx context.Context, ref string) (*coremodel.Component, error) {
	if s == nil || s.Resolver == nil {
		return nil, fmt.Errorf("missing component resolver")
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("missing component ref")
	}
	return s.Resolver.ResolveComponentRef(ctx, ref)
}

func (s *service) resolveLoadedComponent(ctx context.Context, id modeluuid.UUID) (*component.Loaded, error) {
	if s == nil || s.Resolver == nil {
		return nil, fmt.Errorf("missing component resolver")
	}
	return s.Resolver.ResolveComponent(ctx, id)
}
