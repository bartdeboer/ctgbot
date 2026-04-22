package chatbroker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/messenger"
)

func (b *Broker) HandleIncomingUpdate(ctx context.Context, u messenger.InboundPayload) (messenger.OutboundPayload, error) {
	text := strings.TrimSpace(u.Text.Text)
	if text == "" && len(u.Attachments) == 0 {
		return messenger.OutboundPayload{}, nil
	}

	msg := messenger.IncomingMessage{
		ProviderType:      strings.TrimSpace(u.ProviderType),
		ProviderChatID:    strings.TrimSpace(u.ProviderChatID),
		ProviderThreadID:  strings.TrimSpace(u.ProviderThreadID),
		Message:           text,
		ChatLabel:         strings.TrimSpace(u.ChatLabel),
		UserLabel:         strings.TrimSpace(u.UserLabel),
		UserID:            u.UserID,
		IsAdmin:           u.IsAdmin,
		ProviderMessageID: strings.TrimSpace(u.ProviderMessageID),
	}

	var savedPaths []string
	if len(u.Attachments) > 0 {
		attachments := make([]messenger.IncomingAttachment, 0, len(u.Attachments))
		for _, attachment := range u.Attachments {
			attachments = append(attachments, messenger.IncomingAttachment{
				Kind:     strings.TrimSpace(attachment.Kind),
				Filename: strings.TrimSpace(attachment.Filename),
				Content:  append([]byte(nil), attachment.Content...),
			})
		}
		var err error
		savedPaths, err = b.handleIncomingAttachments(ctx, msg, attachments)

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
		msg.Message = text
	}

	result, err := b.HandleIncomingMessage(ctx, msg)

	if err != nil {
		return messenger.OutboundPayload{}, err
	}
	return result, nil
}

func (b *Broker) HandleIncomingMessage(ctx context.Context, msg messenger.IncomingMessage) (messenger.OutboundPayload, error) {
	text := strings.TrimSpace(msg.Message)
	if text == "" {
		return messenger.OutboundPayload{}, nil
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
		reply, err := b.handleCommand(ctx, chatCfg.ID, thread, msg.UserID, msg.IsAdmin, args[0], args[1:])

		if err != nil {
			return payloadResult(fmt.Sprintf("command error: %v", err)), nil
		}
		if strings.TrimSpace(reply) == "" {
			return messenger.OutboundPayload{}, nil
		}
		return payloadResult(reply), nil
	}

	started := false
	startSent := false
	conv, err := b.GetActiveSession(ctx, thread)

	if err != nil {
		return payloadResult(fmt.Sprintf("conversation error: %v", err)), nil
	}
	if conv == nil {
		started = true
		conv, err = b.StartSession(ctx, chatCfg.ID, thread, "", false)

		if err != nil {
			return payloadResult(fmt.Sprintf("conversation error: %v", err)), nil
		}
		if conv != nil {
			if sendErr := b.sendThreadText(ctx, conv, fmt.Sprintf("conversation started\ncontainer: %s\nworkspace: %s", conv.ContainerName(b.Config), conv.WorkspaceHost)); sendErr == nil {
				startSent = true
			}
			thread = conv
		}
	}

	outcome, err := b.HandlePrompt(ctx, chatCfg.ID, thread, text)

	if err != nil {
		return payloadResult(fmt.Sprintf("conversation error: %v", err)), nil
	}

	if started && !startSent && thread != nil {
		// TODO: Send "conversation started" notification message.
	}
	return payloadResult(outcome.Reply), nil
}

func payloadResult(text string) messenger.OutboundPayload {
	return messenger.OutboundPayload{
		Text: messenger.TextMessage{Text: strings.TrimSpace(text)},
	}
}

func (b *Broker) ResolveIncomingThread(ctx context.Context, msg messenger.IncomingMessage, create bool) (*appstate.ChatConfigEntry, *Thread, error) {
	return b.resolveIncomingThread(ctx, msg, create)
}

func (b *Broker) resolveIncomingThread(ctx context.Context, msg messenger.IncomingMessage, create bool) (*appstate.ChatConfigEntry, *Thread, error) {
	if b.Config == nil {
		return nil, nil, fmt.Errorf("missing config")
	}
	if b.Sessions == nil {
		return nil, nil, fmt.Errorf("missing session store")
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
		chatLabel = strings.TrimSpace(msg.UserLabel)
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
		thread, err = b.Sessions.EnsureThread(ctx, chatCfg.ID, providerThreadID)
	} else {
		thread, err = b.Sessions.FindThread(ctx, chatCfg.ID, providerThreadID)
	}

	if err != nil {
		return nil, nil, err
	}
	return chatCfg, thread, nil
}

func (b *Broker) handleIncomingAttachments(ctx context.Context, msg messenger.IncomingMessage, attachments []messenger.IncomingAttachment) ([]string, error) {
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

	workspaceHost := b.Config.ChatWorkspaceHostPathByID(chatCfg.ID)
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
