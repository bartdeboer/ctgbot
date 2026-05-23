package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type chatCreateCommand struct {
	Label string
}

type chatListCommand struct{}

type chatDroppedCommand struct{}

type chatBindCommand struct {
	Component         string
	ExternalChannelID string
	Label             string
	Role              string
}

type chatWorkspaceSetCommand struct {
	Chat      string
	Workspace string
}

type chatWorkspaceClearCommand struct {
	Chat string
}

type chatComponentAddCommand struct {
	Chat              string
	Role              coremodel.ChatComponentRole
	Component         string
	ExternalChannelID string
}

type chatComponentRemoveCommand struct {
	Chat      string
	Role      coremodel.ChatComponentRole
	Component string
}

type chatComponentListCommand struct {
	Chat string
}

type chatComponentFilterAddCommand struct {
	Chat              string
	Source            string
	Filter            string
	ExternalChannelID string
}

type chatComponentFilterRemoveCommand struct {
	Chat              string
	Source            string
	Filter            string
	ExternalChannelID string
}

type chatComponentFilterClearCommand struct {
	Chat              string
	Source            string
	ExternalChannelID string
}

type chatComponentFilterListCommand struct {
	Chat              string
	Source            string
	ExternalChannelID string
}

func chatCLICommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		cliRootDefinition("chat create <label>", "Create a chat", buildChatCreateCommand),
		cliRootDefinition("chat list", "List chats", func(req *clir.Request) (any, error) { _ = req; return chatListCommand{}, nil }),
		cliRootDefinition("chat dropped", "List unresolved dropped inbound chats", func(req *clir.Request) (any, error) { _ = req; return chatDroppedCommand{}, nil }),
		cliRootDefinition("chat bind <component> <externalChannelID>", "Create an enabled chat for a dropped inbound external channel and bind the inbound component", buildChatBindCommand),
		cliRootDefinition("chat <chatID> workspace set <workspace>", "Assign a named workspace to a chat", buildChatWorkspaceSetCommand),
		cliRootDefinition("chat <chatID> workspace clear", "Clear the named workspace from a chat", buildChatWorkspaceClearCommand),
		cliRootDefinition("chat <chatID> component add <role> <component>", "Bind a registered component to a chat by role", buildChatComponentAddCommand),
		cliRootDefinition("chat <chatID> component remove <role> <component>", "Remove a component binding from a chat by role", buildChatComponentRemoveCommand),
		cliRootDefinition("chat <chatID> component list", "List component bindings for a chat", buildChatComponentListCommand),
		cliRootDefinition("chat <chatID> component <source> filter add <filter>", "Add an inbound event filter for a chat source binding", buildChatComponentFilterAddCommand),
		cliRootDefinition("chat <chatID> component <source> filter remove <filter>", "Remove an inbound event filter from a chat source binding", buildChatComponentFilterRemoveCommand),
		cliRootDefinition("chat <chatID> component <source> filter clear", "Clear inbound event filters for a chat source binding", buildChatComponentFilterClearCommand),
		cliRootDefinition("chat <chatID> component <source> filter list", "List inbound event filters for a chat source binding", buildChatComponentFilterListCommand),
	}
}

func registerChatCLICommandHandlers(registry *commandengine.Registry, surface *cliCommandSurface) error {
	registrations := []func() error{
		func() error { return commandengine.Register[chatCreateCommand](registry, surface.handleChatCreate) },
		func() error { return commandengine.Register[chatListCommand](registry, surface.handleChatList) },
		func() error { return commandengine.Register[chatDroppedCommand](registry, surface.handleChatDropped) },
		func() error { return commandengine.Register[chatBindCommand](registry, surface.handleChatBind) },
		func() error {
			return commandengine.Register[chatWorkspaceSetCommand](registry, surface.handleChatWorkspaceSet)
		},
		func() error {
			return commandengine.Register[chatWorkspaceClearCommand](registry, surface.handleChatWorkspaceClear)
		},
		func() error {
			return commandengine.Register[chatComponentAddCommand](registry, surface.handleChatComponentAdd)
		},
		func() error {
			return commandengine.Register[chatComponentRemoveCommand](registry, surface.handleChatComponentRemove)
		},
		func() error {
			return commandengine.Register[chatComponentListCommand](registry, surface.handleChatComponentList)
		},
		func() error {
			return commandengine.Register[chatComponentFilterAddCommand](registry, surface.handleChatComponentFilterAdd)
		},
		func() error {
			return commandengine.Register[chatComponentFilterRemoveCommand](registry, surface.handleChatComponentFilterRemove)
		},
		func() error {
			return commandengine.Register[chatComponentFilterClearCommand](registry, surface.handleChatComponentFilterClear)
		},
		func() error {
			return commandengine.Register[chatComponentFilterListCommand](registry, surface.handleChatComponentFilterList)
		},
	}
	for _, register := range registrations {
		if err := register(); err != nil {
			return err
		}
	}
	return nil
}

func cliRootDefinition(pattern string, help string, build commandengine.BuildFunc) commandengine.Definition {
	return commandengine.Definition{
		Pattern: pattern,
		Help:    help,
		Build:   build,
		Sources: []commandengine.Source{commandengine.SourceCLI},
		Policy:  simplerbac.Any(simplerbac.RoleRoot),
	}
}

func buildChatCreateCommand(req *clir.Request) (any, error) {
	if err := parseNoFlags("chat create", req); err != nil {
		return nil, err
	}
	label := strings.TrimSpace(req.Params["label"])
	if label == "" {
		return nil, fmt.Errorf("missing chat label")
	}
	return chatCreateCommand{Label: label}, nil
}

func buildChatBindCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("chat bind", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	role := fs.String("role", "", "Binding role override (source, relay, or all)")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	return chatBindCommand{
		Component:         strings.TrimSpace(req.Params["component"]),
		ExternalChannelID: strings.TrimSpace(req.Params["externalChannelID"]),
		Label:             strings.TrimSpace(strings.Join(fs.Args(), " ")),
		Role:              strings.TrimSpace(*role),
	}, nil
}

func buildChatWorkspaceSetCommand(req *clir.Request) (any, error) {
	if err := parseNoFlags("chat workspace set", req); err != nil {
		return nil, err
	}
	return chatWorkspaceSetCommand{Chat: strings.TrimSpace(req.Params["chatID"]), Workspace: strings.TrimSpace(req.Params["workspace"])}, nil
}

func buildChatWorkspaceClearCommand(req *clir.Request) (any, error) {
	if err := parseNoFlags("chat workspace clear", req); err != nil {
		return nil, err
	}
	return chatWorkspaceClearCommand{Chat: strings.TrimSpace(req.Params["chatID"])}, nil
}

func buildChatComponentAddCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("chat component add", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	externalChannelID := fs.String("external-channel-id", "", "External provider channel id for source/relay bindings")
	externalChatID := fs.String("external-chat-id", "", "Deprecated alias for --external-channel-id")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	channelID := strings.TrimSpace(*externalChannelID)
	if channelID == "" {
		channelID = strings.TrimSpace(*externalChatID)
	}
	return chatComponentAddCommand{
		Chat:              strings.TrimSpace(req.Params["chatID"]),
		Role:              coremodel.ChatComponentRole(strings.TrimSpace(req.Params["role"])),
		Component:         strings.TrimSpace(req.Params["component"]),
		ExternalChannelID: channelID,
	}, nil
}

func buildChatComponentRemoveCommand(req *clir.Request) (any, error) {
	if err := parseNoFlags("chat component remove", req); err != nil {
		return nil, err
	}
	return chatComponentRemoveCommand{
		Chat:      strings.TrimSpace(req.Params["chatID"]),
		Role:      coremodel.ChatComponentRole(strings.TrimSpace(req.Params["role"])),
		Component: strings.TrimSpace(req.Params["component"]),
	}, nil
}

func buildChatComponentListCommand(req *clir.Request) (any, error) {
	if err := parseNoFlags("chat component list", req); err != nil {
		return nil, err
	}
	return chatComponentListCommand{Chat: strings.TrimSpace(req.Params["chatID"])}, nil
}

func buildChatComponentFilterAddCommand(req *clir.Request) (any, error) {
	channelID, err := parseExternalChannelFlag("chat component filter add", req)
	if err != nil {
		return nil, err
	}
	return chatComponentFilterAddCommand{Chat: strings.TrimSpace(req.Params["chatID"]), Source: strings.TrimSpace(req.Params["source"]), Filter: strings.TrimSpace(req.Params["filter"]), ExternalChannelID: channelID}, nil
}

func buildChatComponentFilterRemoveCommand(req *clir.Request) (any, error) {
	channelID, err := parseExternalChannelFlag("chat component filter remove", req)
	if err != nil {
		return nil, err
	}
	return chatComponentFilterRemoveCommand{Chat: strings.TrimSpace(req.Params["chatID"]), Source: strings.TrimSpace(req.Params["source"]), Filter: strings.TrimSpace(req.Params["filter"]), ExternalChannelID: channelID}, nil
}

func buildChatComponentFilterClearCommand(req *clir.Request) (any, error) {
	channelID, err := parseExternalChannelFlag("chat component filter clear", req)
	if err != nil {
		return nil, err
	}
	return chatComponentFilterClearCommand{Chat: strings.TrimSpace(req.Params["chatID"]), Source: strings.TrimSpace(req.Params["source"]), ExternalChannelID: channelID}, nil
}

func buildChatComponentFilterListCommand(req *clir.Request) (any, error) {
	channelID, err := parseExternalChannelFlag("chat component filter list", req)
	if err != nil {
		return nil, err
	}
	return chatComponentFilterListCommand{Chat: strings.TrimSpace(req.Params["chatID"]), Source: strings.TrimSpace(req.Params["source"]), ExternalChannelID: channelID}, nil
}

func parseNoFlags(name string, req *clir.Request) error {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs.Parse(req.Extra)
}

func parseExternalChannelFlag(name string, req *clir.Request) (string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	externalChannelID := fs.String("external-channel-id", "", "External provider channel id when the source has multiple bindings in the chat")
	if err := fs.Parse(req.Extra); err != nil {
		return "", err
	}
	return strings.TrimSpace(*externalChannelID), nil
}

func (s *cliCommandSurface) handleChatCreate(ctx context.Context, req commandengine.Request, cmd chatCreateCommand) (commandengine.Result, error) {
	_ = req
	chat, err := s.service.CreateChat(ctx, cmd.Label, "")
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: strings.Join([]string{
		"chat created",
		fmt.Sprintf("id: %s", chat.ID),
		fmt.Sprintf("label: %s", chat.Label),
		fmt.Sprintf("workspace: %s", chat.Workspace),
	}, "\n")}, nil
}

func (s *cliCommandSurface) handleChatList(ctx context.Context, req commandengine.Request, cmd chatListCommand) (commandengine.Result, error) {
	_, _ = req, cmd
	chats, err := s.service.ListChats(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(chats) == 0 {
		return commandengine.Result{Text: "no chats"}, nil
	}
	lines := make([]string, 0, len(chats))
	for _, info := range chats {
		chat := info.Chat
		lines = append(lines, fmt.Sprintf("%s\tshort_id=%s\t%s\tworkspace=%s\tenabled=%t", chat.ID, info.ShortID, chat.Label, chat.Workspace, chat.Enabled))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (s *cliCommandSurface) handleChatDropped(ctx context.Context, req commandengine.Request, cmd chatDroppedCommand) (commandengine.Result, error) {
	_, _ = req, cmd
	drops, err := s.service.ListInboundDrops(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(drops) == 0 {
		return commandengine.Result{Text: "no dropped chats"}, nil
	}
	lines := make([]string, 0, len(drops))
	for _, drop := range drops {
		lines = append(lines, fmt.Sprintf("%s\texternal_channel_id=%s\tmessages=%d\tlast_seen=%s\tlabel=%s\tactor=%s\tpreview=%s", drop.ComponentRef, drop.ExternalChannelID, drop.MessageCount, drop.LastSeenAt.Format(time.RFC3339), drop.ChatLabel, displayActor(drop.ActorLabel, drop.ActorID), drop.LastTextPreview))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (s *cliCommandSurface) handleChatBind(ctx context.Context, req commandengine.Request, cmd chatBindCommand) (commandengine.Result, error) {
	_ = req
	result, err := s.service.BindInboundChat(ctx, cmd.Component, cmd.ExternalChannelID, cmd.Label, cmd.Role)
	if err != nil {
		return commandengine.Result{}, err
	}
	lines := []string{"chat bound", fmt.Sprintf("chat_id: %s", result.Chat.ID), fmt.Sprintf("label: %s", result.Chat.Label)}
	for _, binding := range result.Bindings {
		lines = append(lines, fmt.Sprintf("binding: role=%s component=%s external_channel_id=%s", binding.Role, result.Component.Ref(), binding.ExternalChannelID))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (s *cliCommandSurface) handleChatWorkspaceSet(ctx context.Context, req commandengine.Request, cmd chatWorkspaceSetCommand) (commandengine.Result, error) {
	_ = req
	chatID, err := s.service.ResolveChatRef(ctx, cmd.Chat)
	if err != nil {
		return commandengine.Result{}, fmt.Errorf("resolve chat id: %w", err)
	}
	chat, err := s.service.SetChatWorkspace(ctx, chatID, cmd.Workspace)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: strings.Join([]string{"chat workspace updated", fmt.Sprintf("chat_id: %s", chat.ID), fmt.Sprintf("workspace: %s", chat.Workspace)}, "\n")}, nil
}

func (s *cliCommandSurface) handleChatWorkspaceClear(ctx context.Context, req commandengine.Request, cmd chatWorkspaceClearCommand) (commandengine.Result, error) {
	_ = req
	chatID, err := s.service.ResolveChatRef(ctx, cmd.Chat)
	if err != nil {
		return commandengine.Result{}, fmt.Errorf("resolve chat id: %w", err)
	}
	chat, err := s.service.SetChatWorkspace(ctx, chatID, "")
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: strings.Join([]string{"chat workspace cleared", fmt.Sprintf("chat_id: %s", chat.ID)}, "\n")}, nil
}

func (s *cliCommandSurface) handleChatComponentAdd(ctx context.Context, req commandengine.Request, cmd chatComponentAddCommand) (commandengine.Result, error) {
	_ = req
	chatID, err := s.service.ResolveChatRef(ctx, cmd.Chat)
	if err != nil {
		return commandengine.Result{}, fmt.Errorf("resolve chat id: %w", err)
	}
	result, err := s.service.AddChatComponent(ctx, chatID, cmd.Role, cmd.Component, cmd.ExternalChannelID)
	if err != nil {
		return commandengine.Result{}, err
	}
	lines := []string{"chat component bound", fmt.Sprintf("chat_id: %s", result.Binding.ChatID)}
	if result.ComponentRef != "" {
		lines = append(lines, fmt.Sprintf("component: %s", result.ComponentRef), fmt.Sprintf("runtime: %s", result.Runtime), fmt.Sprintf("home_path: %s", result.HomePath))
	} else {
		lines = append(lines, fmt.Sprintf("component_id: %s", result.Binding.ComponentID))
	}
	lines = append(lines, fmt.Sprintf("role: %s", result.Binding.Role))
	if result.Binding.ExternalChannelID != "" {
		lines = append(lines, fmt.Sprintf("external_channel_id: %s", result.Binding.ExternalChannelID))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (s *cliCommandSurface) handleChatComponentRemove(ctx context.Context, req commandengine.Request, cmd chatComponentRemoveCommand) (commandengine.Result, error) {
	_ = req
	chatID, err := s.service.ResolveChatRef(ctx, cmd.Chat)
	if err != nil {
		return commandengine.Result{}, fmt.Errorf("resolve chat id: %w", err)
	}
	result, err := s.service.RemoveChatComponent(ctx, chatID, cmd.Role, cmd.Component)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !result.Removed {
		return commandengine.Result{Text: strings.Join([]string{"chat component binding not found", fmt.Sprintf("chat_id: %s", chatID), fmt.Sprintf("component: %s", result.ComponentRef), fmt.Sprintf("role: %s", cmd.Role)}, "\n")}, nil
	}
	lines := []string{"chat component binding removed", fmt.Sprintf("chat_id: %s", result.Binding.ChatID), fmt.Sprintf("component: %s", result.ComponentRef), fmt.Sprintf("role: %s", result.Binding.Role)}
	if result.Binding.ExternalChannelID != "" {
		lines = append(lines, fmt.Sprintf("external_channel_id: %s", result.Binding.ExternalChannelID))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (s *cliCommandSurface) handleChatComponentList(ctx context.Context, req commandengine.Request, cmd chatComponentListCommand) (commandengine.Result, error) {
	_ = req
	chatID, err := s.service.ResolveChatRef(ctx, cmd.Chat)
	if err != nil {
		return commandengine.Result{}, fmt.Errorf("resolve chat id: %w", err)
	}
	bindings, err := s.service.ListChatComponents(ctx, chatID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(bindings) == 0 {
		return commandengine.Result{Text: "no component bindings"}, nil
	}
	lines := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		lines = append(lines, fmt.Sprintf("%s\truntime=%s\trole=%s\texternal_channel_id=%s", binding.ComponentRef, binding.Runtime, binding.Binding.Role, binding.Binding.ExternalChannelID))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (s *cliCommandSurface) handleChatComponentFilterAdd(ctx context.Context, req commandengine.Request, cmd chatComponentFilterAddCommand) (commandengine.Result, error) {
	_ = req
	result, err := s.service.AddChatComponentFilter(ctx, cmd.Chat, cmd.Source, cmd.ExternalChannelID, cmd.Filter)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: strings.Join([]string{"chat component filter added", fmt.Sprintf("chat_id: %s", result.Chat.ID), fmt.Sprintf("source: %s", result.Source.Ref()), fmt.Sprintf("external_channel_id: %s", result.SourceBinding.ExternalChannelID), fmt.Sprintf("filter: %s", result.Filter.Ref()), fmt.Sprintf("binding_id: %s", result.Binding.ID)}, "\n")}, nil
}

func (s *cliCommandSurface) handleChatComponentFilterRemove(ctx context.Context, req commandengine.Request, cmd chatComponentFilterRemoveCommand) (commandengine.Result, error) {
	_ = req
	result, err := s.service.RemoveChatComponentFilter(ctx, cmd.Chat, cmd.Source, cmd.ExternalChannelID, cmd.Filter)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: strings.Join([]string{"chat component filter removed", fmt.Sprintf("chat_id: %s", result.Chat.ID), fmt.Sprintf("source: %s", result.Source.Ref()), fmt.Sprintf("external_channel_id: %s", result.SourceBinding.ExternalChannelID), fmt.Sprintf("filter: %s", result.Filter.Ref()), fmt.Sprintf("disabled: %t", result.Disabled)}, "\n")}, nil
}

func (s *cliCommandSurface) handleChatComponentFilterClear(ctx context.Context, req commandengine.Request, cmd chatComponentFilterClearCommand) (commandengine.Result, error) {
	_ = req
	result, err := s.service.ClearChatComponentFilters(ctx, cmd.Chat, cmd.Source, cmd.ExternalChannelID)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: strings.Join([]string{"chat component filter cleared", fmt.Sprintf("chat_id: %s", result.Chat.ID), fmt.Sprintf("source: %s", result.Source.Ref()), fmt.Sprintf("external_channel_id: %s", result.SourceBinding.ExternalChannelID), fmt.Sprintf("disabled: %d", result.Disabled)}, "\n")}, nil
}

func (s *cliCommandSurface) handleChatComponentFilterList(ctx context.Context, req commandengine.Request, cmd chatComponentFilterListCommand) (commandengine.Result, error) {
	_ = req
	result, err := s.service.ListChatComponentFilters(ctx, cmd.Chat, cmd.Source, cmd.ExternalChannelID)
	if err != nil {
		return commandengine.Result{}, err
	}
	lines := []string{"chat component filter list", fmt.Sprintf("chat_id: %s", result.Chat.ID), fmt.Sprintf("source: %s", result.Source.Ref()), fmt.Sprintf("external_channel_id: %s", result.SourceBinding.ExternalChannelID)}
	if len(result.Bindings) == 0 {
		lines = append(lines, "no filters")
		return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
	}
	for _, binding := range result.Bindings {
		lines = append(lines, fmt.Sprintf("filter: %s\tbinding_id=%s", binding.FilterRef, binding.Binding.ID))
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func displayActor(label string, id string) string {
	label = strings.TrimSpace(label)
	id = strings.TrimSpace(id)
	switch {
	case label != "" && id != "" && label != id:
		return label + " (" + id + ")"
	case label != "":
		return label
	default:
		return id
	}
}
