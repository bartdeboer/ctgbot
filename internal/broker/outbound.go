package broker

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	component "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (b *Broker) relayPayloadToRelayBindings(ctx context.Context, relayBindings []RelayBinding, thread coremodel.Thread, payload message.OutboundPayload) error {
	if len(relayBindings) == 0 || payload.IsZero() {
		return nil
	}
	for _, relayBinding := range relayBindings {
		target, ok, err := b.App.RelayTarget(ctx, thread.ID, relayBinding.Binding)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		outbound := payload
		outbound.ProviderChannelID = target.ProviderChannelID
		outbound.ProviderThreadID = target.ProviderThreadID
		if err := relayBinding.Relay.Send(ctx, outbound); err != nil {
			return err
		}
	}
	return nil
}

func (b *Broker) resolveRelayBindingsForChat(ctx context.Context, chatID modeluuid.UUID) ([]RelayBinding, error) {
	chat, err := b.App.Chat(ctx, chatID)
	if err != nil {
		return nil, err
	}
	if chat == nil {
		return nil, fmt.Errorf("chat not found: %s", chatID)
	}
	spec, err := b.runtimeSpec(ctx, *chat)
	if err != nil {
		return nil, err
	}

	relayBindings := make([]RelayBinding, 0, len(spec.Bindings))
	for _, binding := range spec.Bindings {
		if binding.Role != coremodel.ChatComponentRoleRelay {
			continue
		}
		instance := spec.Loaded[binding.ComponentID]
		if instance == nil {
			continue
		}
		relay, ok := instance.Component.(component.OutboundRelay)
		if !ok {
			continue
		}
		relayBindings = append(relayBindings, RelayBinding{
			ComponentID: binding.ComponentID,
			Binding:     binding,
			Relay:       relay,
		})
	}
	return relayBindings, nil
}

func (b *Broker) storeAndRelayMessage(ctx context.Context, runtime *ChatRuntime, chat coremodel.Chat, thread coremodel.Thread, threadMessage coremodel.ThreadMessage, sourceType string) (*coremodel.ThreadMessage, error) {
	return b.storeAndRelayMessageWithPayload(ctx, runtime, chat, thread, threadMessage, sourceType, message.OutboundPayload{})
}

func (b *Broker) storeAndRelayMessageWithPayload(ctx context.Context, runtime *ChatRuntime, chat coremodel.Chat, thread coremodel.Thread, threadMessage coremodel.ThreadMessage, sourceType string, payload message.OutboundPayload) (*coremodel.ThreadMessage, error) {
	threadMessage.ChatID = chat.ID
	threadMessage.ThreadID = thread.ID
	threadMessage.Direction = coremodel.MessageDirectionOutbound
	if threadMessage.Kind == "" {
		threadMessage.Kind = coremodel.MessageKindMessage
	}
	if threadMessage.Role == "" {
		threadMessage.Role = coremodel.MessageRoleAgent
	}
	if strings.TrimSpace(threadMessage.ActorLabel) == "" {
		threadMessage.ActorLabel = sourceType
	}
	if err := b.App.StoreOutboundMessage(ctx, &threadMessage, payload.Attachments); err != nil {
		return nil, err
	}
	if strings.TrimSpace(payload.Text.Text) == "" {
		payload.Text.Text = threadMessage.Text
	}
	payload.Attachments = append([]message.Media(nil), payload.Attachments...)
	if runtime != nil {
		if err := b.relayPayloadToRelayBindings(ctx, runtime.Relays, thread, payload); err != nil {
			return nil, err
		}
	}
	return &threadMessage, nil
}

func (b *Broker) relayOnlyMessage(ctx context.Context, runtime *ChatRuntime, chat coremodel.Chat, thread coremodel.Thread, threadMessage coremodel.ThreadMessage, payload message.OutboundPayload) error {
	_ = chat
	if runtime == nil {
		return nil
	}
	text := strings.TrimSpace(threadMessage.Text)
	if text == "" {
		return nil
	}
	if strings.TrimSpace(payload.Text.Text) == "" {
		payload.Text.Text = text
	}
	return b.relayPayloadToRelayBindings(ctx, runtime.Relays, thread, payload)
}

func (b *Broker) deliverPayload(ctx context.Context, runtime *ChatRuntime, chat coremodel.Chat, thread coremodel.Thread, payload message.OutboundPayload, componentID modeluuid.UUID) ([]coremodel.ThreadMessage, error) {
	if runtime == nil || payload.IsZero() {
		return nil, nil
	}
	message := coremodel.ThreadMessage{
		ChatID:      chat.ID,
		ThreadID:    thread.ID,
		Direction:   coremodel.MessageDirectionOutbound,
		Role:        outboundRole(payload.Role),
		Kind:        outboundKind(payload.Kind),
		ComponentID: componentID,
		Text:        strings.TrimSpace(payload.Text.Text),
	}
	if err := b.App.StoreOutboundMessage(ctx, &message, payload.Attachments); err != nil {
		return nil, err
	}
	if err := b.relayPayloadToRelayBindings(ctx, runtime.Relays, thread, payload); err != nil {
		return nil, err
	}
	return []coremodel.ThreadMessage{message}, nil
}

func materializeIncomingAttachments(workspacePath string, runtimeWorkspacePath string, attachments []message.Media) ([]string, error) {
	if len(attachments) == 0 {
		return nil, nil
	}
	workspacePath = strings.TrimSpace(workspacePath)
	if workspacePath == "" {
		return nil, fmt.Errorf("missing workspace path")
	}
	runtimeWorkspacePath = strings.TrimSpace(runtimeWorkspacePath)
	if runtimeWorkspacePath == "" {
		runtimeWorkspacePath = workspacePath
	}
	inboxHost := filepath.Join(workspacePath, "inbox")
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
		savedPaths = append(savedPaths, filepath.ToSlash(filepath.Join(runtimeWorkspacePath, "inbox", filename)))
	}
	return savedPaths, nil
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
	return strings.TrimSpace(b.String())
}

func (b *Broker) relaySystemMessage(ctx context.Context, runtime *ChatRuntime, chat coremodel.Chat, thread coremodel.Thread, text string) (*coremodel.ThreadMessage, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	threadMessage := &coremodel.ThreadMessage{
		ChatID:      chat.ID,
		ThreadID:    thread.ID,
		Direction:   coremodel.MessageDirectionOutbound,
		Role:        coremodel.MessageRoleSystem,
		Kind:        coremodel.MessageKindMessage,
		ActorID:     "ctgbot",
		ActorLabel:  "ctgbot",
		Text:        text,
		ComponentID: modeluuid.UUID{},
	}
	if err := b.App.StoreOutboundMessage(ctx, threadMessage, nil); err != nil {
		return nil, err
	}
	payload := message.OutboundPayload{Text: message.TextMessage{Text: text}}
	relayBindings := []RelayBinding(nil)
	if runtime != nil {
		relayBindings = runtime.Relays
	} else {
		var err error
		relayBindings, err = b.resolveRelayBindingsForChat(ctx, chat.ID)
		if err != nil {
			return nil, err
		}
	}
	if err := b.relayPayloadToRelayBindings(ctx, relayBindings, thread, payload); err != nil {
		return nil, err
	}
	return threadMessage, nil
}

func outboundRole(role coremodel.MessageRole) coremodel.MessageRole {
	if role != "" {
		return role
	}
	return coremodel.MessageRoleAgent
}

func outboundKind(kind coremodel.MessageKind) coremodel.MessageKind {
	if kind != "" {
		return kind
	}
	return coremodel.MessageKindMessage
}

func conversationErrorText(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	if text == "" {
		return ""
	}
	if strings.Contains(text, "returned an empty response") {
		return text
	}
	return "conversation error: " + text
}
