package broker

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/inbound"
)

type DroppedEventReplayResult struct {
	Drop    coremodel.DroppedEvent
	Outcome EventOutcome
}

func (b *Broker) AllowDropped(ctx context.Context, ref string) (DroppedEventReplayResult, error) {
	if err := b.ensureReady(); err != nil {
		return DroppedEventReplayResult{}, err
	}
	drop, event, err := b.droppedInboundEvent(ctx, ref)
	if err != nil {
		return DroppedEventReplayResult{}, err
	}
	outcome, err := b.handleInbound(ctx, event, inboundRouteOptions{
		bypassEventFilters: true,
	})
	if err != nil {
		return DroppedEventReplayResult{Drop: *drop, Outcome: outcome}, err
	}
	drop.Status = "replayed"
	if err := b.App.SaveDroppedEvent(ctx, drop); err != nil {
		return DroppedEventReplayResult{Drop: *drop, Outcome: outcome}, err
	}
	return DroppedEventReplayResult{Drop: *drop, Outcome: outcome}, nil
}

func (b *Broker) DroppedEvent(ctx context.Context, ref string) (*coremodel.DroppedEvent, error) {
	id, err := b.App.ResolveDroppedEventID(ctx, ref)
	if err != nil {
		return nil, err
	}
	return b.App.DroppedEvent(ctx, id)
}

func (b *Broker) DroppedEvents(ctx context.Context, limit int) ([]coremodel.DroppedEvent, error) {
	return b.App.ListDroppedEvents(ctx, limit)
}

func (b *Broker) droppedInboundEvent(ctx context.Context, ref string) (*coremodel.DroppedEvent, component.InboundEvent, error) {
	drop, err := b.DroppedEvent(ctx, ref)
	if err != nil {
		return nil, component.InboundEvent{}, err
	}
	if drop == nil || drop.ID.IsNull() {
		return nil, component.InboundEvent{}, fmt.Errorf("dropped event not found: %s", strings.TrimSpace(ref))
	}
	var event component.InboundEvent
	if err := json.Unmarshal([]byte(drop.EventJSON), &event); err != nil {
		return nil, component.InboundEvent{}, fmt.Errorf("decode dropped event: %w", err)
	}
	if event.ComponentID.IsNull() {
		event.ComponentID = drop.ComponentID
	}
	return drop, event, nil
}

func FormatDroppedEventList(ctx context.Context, app App, drops []coremodel.DroppedEvent) string {
	if len(drops) == 0 {
		return "no dropped messages"
	}
	lines := []string{"dropped messages:"}
	for _, drop := range drops {
		id := strings.TrimSpace(drop.ID.String())
		if app != nil {
			id = app.DropNoticeID(ctx, &drop)
		}
		parts := []string{
			id,
			strings.TrimSpace(drop.Status),
			strings.TrimSpace(drop.Reason),
		}
		if sender := strings.TrimSpace(drop.SenderLabel); sender != "" {
			parts = append(parts, "from="+sender)
		}
		if subject := strings.TrimSpace(drop.Subject); subject != "" {
			parts = append(parts, "subject="+subject)
		}
		if preview := strings.TrimSpace(drop.Preview); preview != "" {
			parts = append(parts, "preview="+inbound.Preview(preview, 80))
		}
		lines = append(lines, "- "+strings.Join(nonEmpty(parts), " "))
	}
	return strings.Join(lines, "\n")
}

func FormatDroppedEventView(drop coremodel.DroppedEvent) string {
	var lines []string
	lines = append(lines, "dropped message")
	lines = append(lines, "id: "+drop.ID.String())
	lines = append(lines, "status: "+strings.TrimSpace(drop.Status))
	lines = append(lines, "reason: "+strings.TrimSpace(drop.Reason))
	lines = append(lines, "created_at: "+drop.CreatedAt.Format(time.RFC3339))
	if sender := strings.TrimSpace(drop.SenderLabel); sender != "" {
		lines = append(lines, "from: "+sender)
	}
	if subject := strings.TrimSpace(drop.Subject); subject != "" {
		lines = append(lines, "subject: "+subject)
	}
	if strings.TrimSpace(drop.ProviderChannelID) != "" {
		lines = append(lines, "provider_channel_id: "+strings.TrimSpace(drop.ProviderChannelID))
	}
	if strings.TrimSpace(drop.ProviderThreadID) != "" {
		lines = append(lines, "provider_thread_id: "+strings.TrimSpace(drop.ProviderThreadID))
	}
	if event, ok := decodedDroppedInboundEvent(drop); ok {
		body := strings.TrimRight(event.Payload.Text.Text, "\n")
		if body != "" {
			lines = append(lines, "", "message:", body)
		}
		if len(event.Payload.Attachments) > 0 {
			lines = append(lines, "", "attachments:")
			for _, attachment := range event.Payload.Attachments {
				name := strings.TrimSpace(attachment.Filename)
				if name == "" {
					name = "(unnamed)"
				}
				lines = append(lines, "- "+name)
			}
		}
	}
	return strings.Join(lines, "\n")
}

func decodedDroppedInboundEvent(drop coremodel.DroppedEvent) (component.InboundEvent, bool) {
	if strings.TrimSpace(drop.EventJSON) == "" {
		return component.InboundEvent{}, false
	}
	var event component.InboundEvent
	if err := json.Unmarshal([]byte(drop.EventJSON), &event); err != nil {
		return component.InboundEvent{}, false
	}
	return event, true
}

func nonEmpty(values []string) []string {
	out := values[:0]
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func (b *Broker) DroppedList(ctx context.Context, limit int) (string, error) {
	drops, err := b.DroppedEvents(ctx, limit)
	if err != nil {
		return "", err
	}
	return FormatDroppedEventList(ctx, b.App, drops), nil
}

func (b *Broker) DroppedView(ctx context.Context, ref string) (string, error) {
	drop, err := b.DroppedEvent(ctx, ref)
	if err != nil {
		return "", err
	}
	if drop == nil {
		return "", fmt.Errorf("dropped event not found: %s", strings.TrimSpace(ref))
	}
	return FormatDroppedEventView(*drop), nil
}

func (b *Broker) DroppedAllow(ctx context.Context, ref string) (string, error) {
	result, err := b.AllowDropped(ctx, ref)
	if err != nil {
		return "", err
	}
	lines := []string{
		"dropped message replayed",
		"id: " + result.Drop.ID.String(),
		"status: " + strings.TrimSpace(result.Drop.Status),
	}
	if result.Outcome.Inbound != nil {
		lines = append(lines, "thread_id: "+result.Outcome.Inbound.ThreadID.String())
	}
	return strings.Join(lines, "\n"), nil
}
