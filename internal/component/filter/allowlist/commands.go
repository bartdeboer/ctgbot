package allowlist

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/inbound"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

func (c *Component) handleDroppedView(ctx context.Context, req commandengine.Request, cmd droppedViewCommand) (commandengine.Result, error) {
	_ = req
	if c == nil || c.Storage == nil {
		return commandengine.Result{}, fmt.Errorf("missing allowlist storage")
	}
	dropID, err := c.resolveDroppedEventID(ctx, cmd.DropRef)
	if err != nil {
		return commandengine.Result{}, err
	}
	drop, err := c.Storage.DroppedEvents().GetByID(ctx, dropID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if drop == nil {
		return commandengine.Result{}, fmt.Errorf("dropped event not found: %s", strings.TrimSpace(cmd.DropRef))
	}
	return commandengine.Result{Text: formatDroppedEvent(*drop)}, nil
}

func (c *Component) handleWhitelistAdd(ctx context.Context, req commandengine.Request, cmd whitelistAddCommand) (commandengine.Result, error) {
	binding, err := c.currentAllowlistSourceBinding(ctx, req.Context.ChatID)
	if err != nil {
		return commandengine.Result{}, err
	}
	key := inbound.SenderKey(cmd.Sender)
	if key == "" {
		return commandengine.Result{}, fmt.Errorf("missing sender")
	}
	sender := &coremodel.AllowlistSender{
		SourceBindingID: binding.ID,
		SenderKey:       key,
		SenderLabel:     strings.TrimSpace(cmd.Sender),
	}
	if sender.SenderLabel == "" {
		sender.SenderLabel = key
	}
	if err := c.Storage.AllowlistSenders().Save(ctx, sender); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: "allowlist sender whitelisted\nsender: " + key}, nil
}

func (c *Component) handleWhitelistList(ctx context.Context, req commandengine.Request, cmd whitelistListCommand) (commandengine.Result, error) {
	_ = cmd
	binding, err := c.currentAllowlistSourceBinding(ctx, req.Context.ChatID)
	if err != nil {
		return commandengine.Result{}, err
	}
	senders, err := c.Storage.AllowlistSenders().ListBySourceBindingID(ctx, binding.ID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(senders) == 0 {
		return commandengine.Result{Text: "allowlist whitelist empty"}, nil
	}
	lines := []string{"allowlist whitelist"}
	for _, sender := range senders {
		line := "- " + strings.TrimSpace(sender.SenderKey)
		if label := strings.TrimSpace(sender.SenderLabel); label != "" && label != sender.SenderKey {
			line += " " + label
		}
		lines = append(lines, line)
	}
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func (c *Component) handleWhitelistRemove(ctx context.Context, req commandengine.Request, cmd whitelistRemoveCommand) (commandengine.Result, error) {
	binding, err := c.currentAllowlistSourceBinding(ctx, req.Context.ChatID)
	if err != nil {
		return commandengine.Result{}, err
	}
	key := inbound.SenderKey(cmd.Sender)
	if key == "" {
		return commandengine.Result{}, fmt.Errorf("missing sender")
	}
	deleted, err := c.Storage.AllowlistSenders().DeleteBySourceBindingAndSenderKey(ctx, binding.ID, key)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !deleted {
		return commandengine.Result{Text: "allowlist sender not found\nsender: " + key}, nil
	}
	return commandengine.Result{Text: "allowlist sender removed\nsender: " + key}, nil
}

func (c *Component) resolveDroppedEventID(ctx context.Context, ref string) (modeluuid.UUID, error) {
	ids, err := c.Storage.DroppedEvents().ListIDs(ctx)
	if err != nil {
		return modeluuid.Nil, err
	}
	resolver := repository.NewShortIDResolver(ids)
	id, err := resolver.Resolve(ref)
	if err == nil {
		return id, nil
	}
	var ambiguous *repository.ShortIDAmbiguousError
	if errors.As(err, &ambiguous) {
		return modeluuid.Nil, ambiguousDroppedEventIDError(resolver, ambiguous.Ref, ambiguous.Candidates)
	}
	return modeluuid.Nil, fmt.Errorf("dropped event not found: %s", strings.TrimSpace(ref))
}

func ambiguousDroppedEventIDError(resolver *repository.ShortIDResolver, ref string, candidates []modeluuid.UUID) error {
	lines := []string{"dropped event id " + strings.TrimSpace(ref) + " is ambiguous", "candidates:"}
	for _, candidate := range candidates {
		shortID := candidate.String()
		if resolver != nil {
			if value, err := resolver.ShortIDFor(candidate, 6); err == nil {
				shortID = value
			}
		}
		lines = append(lines, "- "+shortID+" "+candidate.String())
	}
	return errors.New(strings.Join(lines, "\n"))
}

func (c *Component) currentAllowlistSourceBinding(ctx context.Context, chatID modeluuid.UUID) (*coremodel.ChatComponent, error) {
	if c == nil || c.Storage == nil {
		return nil, fmt.Errorf("missing allowlist storage")
	}
	if chatID.IsNull() {
		return nil, fmt.Errorf("missing chat id")
	}
	bindings, err := c.Storage.ChatComponents().ListEnabledByChatID(ctx, chatID)
	if err != nil {
		return nil, err
	}
	candidates := map[modeluuid.UUID]coremodel.ChatComponent{}
	for _, binding := range bindings {
		if binding.Role != coremodel.ChatComponentRoleSource {
			continue
		}
		enabled, err := c.sourceBindingHasAllowlistFilter(ctx, binding.ID)
		if err != nil {
			return nil, err
		}
		if enabled {
			candidates[binding.ID] = binding
		}
	}
	switch len(candidates) {
	case 0:
		return nil, fmt.Errorf("no allowlist filter configured for this chat")
	case 1:
		for _, binding := range candidates {
			return &binding, nil
		}
	}
	keys := make([]string, 0, len(candidates))
	for _, binding := range candidates {
		keys = append(keys, binding.ExternalChannelID)
	}
	sort.Strings(keys)
	return nil, fmt.Errorf("multiple allowlist source bindings configured for this chat: %s", strings.Join(keys, ", "))
}

func (c *Component) sourceBindingHasAllowlistFilter(ctx context.Context, sourceBindingID modeluuid.UUID) (bool, error) {
	bindings, err := c.Storage.InboundFilterBindings().ListEnabledBySourceBindingID(ctx, sourceBindingID)
	if err != nil {
		return false, err
	}
	for _, binding := range bindings {
		registration, err := c.Storage.Components().GetByID(ctx, binding.FilterComponentID)
		if err != nil {
			return false, err
		}
		if IsRegistration(registration) {
			return true, nil
		}
	}
	return false, nil
}

func formatDroppedEvent(drop coremodel.DroppedEvent) string {
	lines := []string{
		"dropped event",
		"id: " + drop.ID.String(),
		"status: " + strings.TrimSpace(drop.Status),
		"action: " + strings.TrimSpace(drop.Action),
		"reason: " + strings.TrimSpace(drop.Reason),
		"component_id: " + drop.ComponentID.String(),
		"provider_channel_id: " + strings.TrimSpace(drop.ProviderChannelID),
		"provider_thread_id: " + strings.TrimSpace(drop.ProviderThreadID),
		"provider_message_id: " + strings.TrimSpace(drop.ProviderMessageID),
		"sender: " + strings.TrimSpace(drop.SenderKey),
	}
	if label := strings.TrimSpace(drop.SenderLabel); label != "" && label != drop.SenderKey {
		lines = append(lines, "sender_label: "+label)
	}
	if subject := strings.TrimSpace(drop.Subject); subject != "" {
		lines = append(lines, "subject: "+subject)
	}
	if preview := strings.TrimSpace(drop.Preview); preview != "" {
		lines = append(lines, "preview: "+preview)
	}
	if event, ok := droppedInboundEvent(drop); ok {
		text := strings.TrimRight(event.Payload.Text.Text, "\n")
		if text != "" {
			lines = append(lines, "", "message:", text)
		}
		if len(event.Payload.Attachments) > 0 {
			lines = append(lines, "", "attachments:")
			for _, attachment := range event.Payload.Attachments {
				label := strings.TrimSpace(attachment.Filename)
				if label == "" {
					label = strings.TrimSpace(attachment.Kind)
				}
				if label == "" {
					label = "attachment"
				}
				if contentType := strings.TrimSpace(attachment.ContentType); contentType != "" {
					label += " (" + contentType + ")"
				}
				lines = append(lines, "- "+label)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func droppedInboundEvent(drop coremodel.DroppedEvent) (component.InboundEvent, bool) {
	if strings.TrimSpace(drop.EventJSON) == "" {
		return component.InboundEvent{}, false
	}
	var event component.InboundEvent
	if err := json.Unmarshal([]byte(drop.EventJSON), &event); err != nil {
		return component.InboundEvent{}, false
	}
	return event, true
}

func unknownSenderNotice(event component.InboundEvent, senderKey string, senderLabel string) string {
	subject := inbound.SubjectFromText(event.Payload.Text.Text)
	lines := []string{
		"Received message from unknown sender.",
		"From: " + displaySender(senderKey, senderLabel),
	}
	if subject != "" {
		lines = append(lines, "Subject: "+subject)
	}
	lines = append(lines, "Drop ID: "+dropIDPlaceholder)
	if providerMessageID := strings.TrimSpace(event.Payload.ProviderMessageID); providerMessageID != "" {
		lines = append(lines, "Provider Message ID: "+providerMessageID)
	}
	lines = append(lines,
		"",
		"/allowlist dropped view "+dropIDPlaceholder,
		"/allowlist whitelist "+strings.TrimSpace(senderKey),
	)
	return strings.Join(lines, "\n")
}

func displaySender(senderKey string, senderLabel string) string {
	senderKey = strings.TrimSpace(senderKey)
	senderLabel = strings.TrimSpace(senderLabel)
	switch {
	case senderLabel == "":
		return senderKey
	case senderKey == "" || strings.EqualFold(senderKey, senderLabel):
		return senderLabel
	case strings.Contains(strings.ToLower(senderLabel), strings.ToLower(senderKey)):
		return senderLabel
	default:
		return senderLabel + " <" + senderKey + ">"
	}
}
