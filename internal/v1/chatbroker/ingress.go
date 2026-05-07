package chatbroker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	configschema "github.com/bartdeboer/ctgbot/internal/schema/config"
	"github.com/bartdeboer/ctgbot/internal/schema/routers"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func (b *Broker) HandleInboundPayload(ctx context.Context, msg messenger.InboundPayload) (messenger.OutboundPayload, error) {
	text := strings.TrimSpace(msg.Text.Text)
	if text == "" && len(msg.Attachments) == 0 {
		return messenger.OutboundPayload{}, nil
	}

	var savedPaths []string
	if len(msg.Attachments) > 0 {
		var err error
		savedPaths, err = b.handleIncomingAttachments(ctx, msg, msg.Attachments)
		if err != nil {
			return messenger.OutboundPayload{}, err
		}
	}

	if text == "" {
		if len(savedPaths) == 0 {
			return messenger.OutboundPayload{}, nil
		}
		return payloadResult(uploadSavedMessage(savedPaths)), nil
	}
	if len(savedPaths) > 0 {
		text = injectFilesIntoPrompt(savedPaths, text)
		msg.Text = messenger.TextMessage{Text: text}
	}

	chatCfg, thread, err := b.resolveIncomingThread(ctx, msg, true)

	if err != nil {
		return messenger.OutboundPayload{}, err
	}
	if chatCfg == nil {
		return messenger.OutboundPayload{}, fmt.Errorf("missing chat mapping")
	}
	if !chatCfg.Enabled {
		b.logf("ignoring update from disabled chat provider=%q chat=%q title=%q", msg.ProviderType, msg.ProviderChatID, chatCfg.ProviderChatTitle)
		return messenger.OutboundPayload{}, nil
	}

	if strings.HasPrefix(text, "/") {
		args := normalizeIncomingCommand(msg.ProviderType, text)
		if len(args) == 0 {
			return messenger.OutboundPayload{}, nil
		}
		if args[0] == "help" {
			return payloadResult(commandHelp(routers.MessageDefinitions())), nil
		}
		engine, err := b.messageCommandEngine()
		if err != nil {
			return payloadResult(fmt.Sprintf("command error: %v", err)), nil
		}
		result, err := engine.Run(ctx, commandengine.Request{
			Context: commandengine.Context{
				ChatID:   chatCfg.ID,
				ThreadID: threadIDOrNil(thread),
				Actor:    b.messageActor(chatCfg.ID, msg),
			},
		}, args)
		if err != nil {
			return payloadResult(fmt.Sprintf("command error: %v", err)), nil
		}
		reply := strings.TrimSpace(result.Text)
		if strings.TrimSpace(reply) == "" {
			return messenger.OutboundPayload{}, nil
		}
		return payloadResult(reply), nil
	}

	conv, err := b.GetActiveSession(ctx, thread)

	if err != nil {
		return payloadResult(fmt.Sprintf("conversation error: %v", err)), nil
	}
	if conv == nil {
		conv, err = b.StartSession(ctx, chatCfg.ID, thread, "", false)

		if err != nil {
			return payloadResult(fmt.Sprintf("conversation error: %v", err)), nil
		}
		if conv != nil {
			thread = conv
		}
	}

	outcome, err := b.HandlePrompt(ctx, chatCfg.ID, thread, text)

	if err != nil {
		return payloadResult(fmt.Sprintf("conversation error: %v", err)), nil
	}

	return payloadResult(outcome.Reply), nil
}

func (b *Broker) messageCommandEngine() (*commandengine.Engine, error) {
	registry, err := configschema.Registry(b.Config)
	if err != nil {
		return nil, err
	}
	handlers := NewCommandHandlers(b)
	return routers.NewMessageCommandEngine(configengine.New(registry), handlers, handlers)
}

func (b *Broker) messageActor(chatID modeluuid.UUID, msg messenger.InboundPayload) commandengine.Actor {
	actor := msg.ResolvedActor()
	roles := append([]simplerbac.Role(nil), actor.Roles...)
	if len(roles) == 0 {
		roles = []simplerbac.Role{simplerbac.RoleUser}
	}
	if b != nil && b.Config != nil && !chatID.IsNull() && b.Config.Chat(chatID).ProcessToolsEnabled() && !actor.HasRole(simplerbac.RoleRoot) {
		roles = append(roles, simplerbac.RoleElevated)
	}
	actorID := strings.TrimSpace(actor.ID)
	if actorID == "" {
		actorID = "user"
	}
	return commandengine.Actor{ID: actorID, Roles: roles}
}

func commandHelp(definitions []commandengine.Definition) string {
	var lines []string
	for _, definition := range definitions {
		for _, route := range definition.Routes() {
			pattern := "/" + commandengine.NormalizePattern(route.Pattern)
			if strings.TrimSpace(definition.Help) == "" {
				lines = append(lines, pattern)
				continue
			}
			lines = append(lines, pattern+" - "+strings.TrimSpace(definition.Help))
		}
	}
	return "Commands:\n" + strings.Join(lines, "\n")
}

func payloadResult(text string) messenger.OutboundPayload {
	return messenger.OutboundPayload{
		Text: messenger.TextMessage{Text: strings.TrimSpace(text)},
	}
}

func (b *Broker) resolveIncomingThread(ctx context.Context, msg messenger.InboundPayload, create bool) (*appstate.ChatConfigEntry, *Thread, error) {
	if b.Config == nil {
		return nil, nil, fmt.Errorf("missing config")
	}
	threads := b.threads()
	if threads == nil {
		return nil, nil, fmt.Errorf("missing storage")
	}

	providerType := strings.TrimSpace(msg.ProviderType)
	providerChatID := strings.TrimSpace(msg.ProviderChatID)
	providerThreadID := strings.TrimSpace(msg.ProviderThreadID)

	if providerType == "" {
		return nil, nil, fmt.Errorf("missing provider type")
	}
	if providerChatID == "" {
		return nil, nil, fmt.Errorf("missing provider chat id")
	}
	if providerThreadID == "" {
		return nil, nil, fmt.Errorf("missing provider thread id")
	}

	chatLabel := strings.TrimSpace(msg.ChatLabel)
	if chatLabel == "" {
		chatLabel = strings.TrimSpace(msg.ResolvedActor().Label)
	}

	var (
		chatCfg *appstate.ChatConfigEntry
		err     error
	)
	if create {
		chatCfg, err = b.Config.EnsureProviderChat(providerType, providerChatID, chatLabel)
	} else {
		chatCfg, err = b.Config.FindProviderChat(providerType, providerChatID)
	}
	if err != nil || chatCfg == nil {
		return chatCfg, nil, err
	}

	var thread *Thread
	if create {
		thread, err = threads.EnsureProviderThread(ctx, chatCfg.ID, providerThreadID)
	} else {
		thread, err = threads.GetByProviderThreadID(ctx, chatCfg.ID, providerThreadID)
	}

	if err != nil {
		return nil, nil, err
	}
	return chatCfg, thread, nil
}

func (b *Broker) handleIncomingAttachments(ctx context.Context, msg messenger.InboundPayload, attachments []messenger.Media) ([]string, error) {
	if len(attachments) == 0 {
		return nil, nil
	}

	chatCfg, _, err := b.resolveIncomingThread(ctx, msg, true)

	if err != nil {
		return nil, err
	}
	if chatCfg == nil {
		return nil, fmt.Errorf("missing chat mapping")
	}
	if !chatCfg.Enabled {
		b.logf("ignoring attachment upload from disabled chat provider=%q chat=%q title=%q", msg.ProviderType, msg.ProviderChatID, chatCfg.ProviderChatTitle)
		return nil, nil
	}

	workspaceHost := b.Config.Chat(chatCfg.ID).WorkspaceHostPath()
	inboxHost := filepath.Join(workspaceHost, "inbox")
	if err := os.MkdirAll(inboxHost, 0o755); err != nil {
		return nil, err
	}

	savedPaths := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		filename := safeIncomingFilename(attachment.Filename)
		targetHost := filepath.Join(inboxHost, filename)
		if err := os.WriteFile(targetHost, attachment.Content, 0o644); err != nil {
			return nil, err
		}
		savedPaths = append(savedPaths, fmt.Sprintf("/workspace/inbox/%s", filename))
	}
	return savedPaths, nil
}

func normalizeIncomingCommand(providerType string, text string) []string {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return nil
	}

	fields[0] = strings.TrimPrefix(fields[0], "/")
	if providerType == "telegram" {
		if i := strings.Index(fields[0], "@"); i >= 0 {
			fields[0] = fields[0][:i]
		}
	}
	return fields
}

func safeIncomingFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return "upload.bin"
	}
	return name
}

func uploadSavedMessage(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	if len(paths) == 1 {
		return "upload saved: " + paths[0]
	}
	return "uploads saved:\n- " + strings.Join(paths, "\n- ")
}

func injectFilesIntoPrompt(paths []string, prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if len(paths) == 0 {
		return prompt
	}
	var b strings.Builder
	b.WriteString("Files made available:\n")
	for _, path := range paths {
		b.WriteString("- ")
		b.WriteString(path)
		b.WriteString("\n")
	}
	if prompt != "" {
		b.WriteString("\n")
		b.WriteString(prompt)
	}
	return b.String()
}
