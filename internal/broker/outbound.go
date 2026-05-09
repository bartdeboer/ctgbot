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

func (b *Broker) storeInboundMessage(ctx context.Context, inbound component.ResolvedInbound) (*coremodel.ThreadMessage, error) {
	actor := inbound.Payload.ResolvedActor()
	metadata := inboundMetadataJSON(inbound.Payload)
	if len(inbound.Metadata) > 0 {
		if metadata != "" {
			metadata += "\n"
		}
		metadata += strings.Join(inbound.Metadata, "\n")
	}
	message := &coremodel.ThreadMessage{
		ChatID:       inbound.Chat.ID,
		ThreadID:     inbound.Thread.ID,
		Direction:    coremodel.MessageDirectionInbound,
		Kind:         inboundKind(inbound),
		ComponentID:  inbound.ComponentID,
		ExternalID:   strings.TrimSpace(inbound.ExternalID),
		ActorLabel:   strings.TrimSpace(actor.Label),
		Text:         strings.TrimSpace(inbound.Payload.Text.Text),
		MetadataJSON: metadata,
	}
	if strings.TrimSpace(actor.ID) != "" {
		message.ActorID = strings.TrimSpace(actor.ID)
	}
	if err := b.Storage.Messages().Append(ctx, message); err != nil {
		return nil, err
	}
	for _, media := range inbound.Payload.Attachments {
		artifact := &coremodel.Artifact{
			ChatID:      inbound.Chat.ID,
			ThreadID:    inbound.Thread.ID,
			MessageID:   message.ID,
			ComponentID: inbound.ComponentID,
			Filename:    strings.TrimSpace(media.Filename),
			ContentType: strings.TrimSpace(media.ContentType),
			Syntax:      strings.TrimSpace(media.Syntax),
			Content:     append([]byte(nil), media.Content...),
		}
		if err := b.Storage.Artifacts().Append(ctx, artifact); err != nil {
			return nil, err
		}
	}
	return message, nil
}

func inboundKind(inbound component.ResolvedInbound) coremodel.MessageKind {
	if strings.TrimSpace(inbound.Payload.Text.Text) != "" {
		return coremodel.MessageKindUser
	}
	return coremodel.MessageKindEvent
}

func (b *Broker) storeMessageArtifacts(ctx context.Context, message *coremodel.ThreadMessage, attachments []message.Media) error {
	if message == nil {
		return fmt.Errorf("missing message")
	}
	for _, media := range attachments {
		if err := b.Storage.Artifacts().Append(ctx, &coremodel.Artifact{
			ChatID:      message.ChatID,
			ThreadID:    message.ThreadID,
			MessageID:   message.ID,
			ComponentID: message.ComponentID,
			Filename:    strings.TrimSpace(media.Filename),
			ContentType: strings.TrimSpace(media.ContentType),
			Syntax:      strings.TrimSpace(media.Syntax),
			Content:     append([]byte(nil), media.Content...),
		}); err != nil {
			return err
		}
	}
	return nil
}

func (b *Broker) storeOutboundMessage(ctx context.Context, message *coremodel.ThreadMessage, attachments []message.Media) error {
	if message == nil {
		return fmt.Errorf("missing message")
	}
	if err := b.Storage.Messages().Append(ctx, message); err != nil {
		return err
	}
	return b.storeMessageArtifacts(ctx, message, attachments)
}

func (b *Broker) relayPayloadToRelayBindings(ctx context.Context, relayBindings []RelayBinding, thread coremodel.Thread, payload message.OutboundPayload) error {
	if len(relayBindings) == 0 || payload.IsZero() {
		return nil
	}
	for _, relayBinding := range relayBindings {
		target, ok, err := b.Mapper.RelayTarget(ctx, thread.ID, relayBinding.Binding)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		outbound := payload
		outbound.ProviderChatID = target.ProviderChatID
		outbound.ProviderThreadID = target.ProviderThreadID
		if err := relayBinding.Relay.Send(ctx, outbound); err != nil {
			return err
		}
	}
	return nil
}

func (b *Broker) resolveRelayBindingsForChat(ctx context.Context, chatID modeluuid.UUID) ([]RelayBinding, error) {
	bindings, err := b.Storage.ChatComponents().ListEnabledByChatID(ctx, chatID)
	if err != nil {
		return nil, err
	}
	relayBindings := make([]RelayBinding, 0, len(bindings))
	for _, binding := range bindings {
		if binding.Role != coremodel.ChatComponentRoleRelay {
			continue
		}
		instance, err := b.Resolver.ResolveComponent(ctx, binding.ComponentID)
		if err != nil {
			return nil, err
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
	threadMessage.ChatID = chat.ID
	threadMessage.ThreadID = thread.ID
	threadMessage.Direction = coremodel.MessageDirectionOutbound
	if threadMessage.Kind == "" {
		threadMessage.Kind = coremodel.MessageKindAgent
	}
	if strings.TrimSpace(threadMessage.ActorLabel) == "" {
		threadMessage.ActorLabel = sourceType
	}
	if err := b.storeOutboundMessage(ctx, &threadMessage, nil); err != nil {
		return nil, err
	}
	payload := message.OutboundPayload{
		Text: message.TextMessage{Text: threadMessage.Text},
	}
	if runtime != nil {
		if err := b.relayPayloadToRelayBindings(ctx, runtime.Relays, thread, payload); err != nil {
			return nil, err
		}
	}
	return &threadMessage, nil
}

func (b *Broker) deliverPayload(ctx context.Context, runtime *ChatRuntime, chat coremodel.Chat, thread coremodel.Thread, payload message.OutboundPayload, componentID modeluuid.UUID) ([]coremodel.ThreadMessage, error) {
	if runtime == nil || payload.IsZero() {
		return nil, nil
	}
	message := coremodel.ThreadMessage{
		ChatID:      chat.ID,
		ThreadID:    thread.ID,
		Direction:   coremodel.MessageDirectionOutbound,
		Kind:        coremodel.MessageKindAgent,
		ComponentID: componentID,
		Text:        strings.TrimSpace(payload.Text.Text),
	}
	if err := b.storeOutboundMessage(ctx, &message, payload.Attachments); err != nil {
		return nil, err
	}
	if err := b.relayPayloadToRelayBindings(ctx, runtime.Relays, thread, payload); err != nil {
		return nil, err
	}
	return []coremodel.ThreadMessage{message}, nil
}

func inboundMetadataJSON(payload message.InboundPayload) string {
	actor := payload.ResolvedActor()
	var metadata []string
	if payload.ProviderType != "" {
		metadata = append(metadata, "provider="+strings.TrimSpace(payload.ProviderType))
	}
	if payload.ProviderChatID != "" {
		metadata = append(metadata, "chat="+strings.TrimSpace(payload.ProviderChatID))
	}
	if payload.ProviderThreadID != "" {
		metadata = append(metadata, "thread="+strings.TrimSpace(payload.ProviderThreadID))
	}
	if payload.ProviderMessageID != "" {
		metadata = append(metadata, "message="+strings.TrimSpace(payload.ProviderMessageID))
	}
	if strings.TrimSpace(actor.ID) != "" {
		metadata = append(metadata, "actor_id="+strings.TrimSpace(actor.ID))
	}
	if strings.TrimSpace(actor.Label) != "" {
		metadata = append(metadata, "actor_label="+strings.TrimSpace(actor.Label))
	}
	if len(actor.Roles) > 0 {
		roles := make([]string, 0, len(actor.Roles))
		for _, role := range actor.Roles {
			if strings.TrimSpace(string(role)) == "" {
				continue
			}
			roles = append(roles, strings.TrimSpace(string(role)))
		}
		if len(roles) > 0 {
			metadata = append(metadata, "actor_roles="+strings.Join(roles, ","))
		}
	}
	return strings.Join(metadata, "\n")
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
		Kind:        coremodel.MessageKindSystem,
		ActorID:     "ctgbot",
		ActorLabel:  "ctgbot",
		Text:        text,
		ComponentID: modeluuid.UUID{},
	}
	if err := b.storeOutboundMessage(ctx, threadMessage, nil); err != nil {
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

func conversationErrorText(err error) string {
	if err == nil {
		return ""
	}
	text := strings.TrimSpace(err.Error())
	if text == "" {
		return ""
	}
	return "conversation error: " + text
}
