package broker

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

const wakeupProviderType = "wakeup"

func (b *Broker) ThreadBusy(threadID modeluuid.UUID) bool {
	if b == nil || b.Turns == nil {
		return false
	}
	// This reports turns actively holding the gate. It intentionally does not
	// inspect queued inbound goroutines; v1 coalesces only while a turn is
	// running, not while work is merely about to queue.
	return b.Turns.Busy(threadID)
}

func (b *Broker) DeliverWake(ctx context.Context, threadID modeluuid.UUID, text string) error {
	if err := b.ensureReady(); err != nil {
		return err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("missing wake text")
	}
	thread, err := b.App.Thread(ctx, threadID)
	if err != nil {
		return err
	}
	if thread == nil {
		return fmt.Errorf("missing wake thread: %s", threadID)
	}
	chat, err := b.App.Chat(ctx, thread.ChatID)
	if err != nil {
		return err
	}
	if chat == nil {
		return fmt.Errorf("missing wake chat: %s", thread.ChatID)
	}
	if !chat.Enabled {
		return fmt.Errorf("wake target chat is disabled: %s", chat.ID)
	}
	// QueueResolvedInbound is accept-and-queue: once accepted, timed intent
	// delivery records success before the asynchronous turn necessarily
	// completes. That is the deliberate v1 tradeoff; recurring heartbeats and
	// idempotent wake reasons make at-least-once bookkeeping unnecessary here.
	actor := coremodel.Actor{ID: "wakeup", Label: "wakeup", Roles: []simplerbac.Role{simplerbac.RoleUser}}
	return b.QueueResolvedInbound(ctx, component.ResolvedInbound{
		Chat:   *chat,
		Thread: *thread,
		Payload: message.InboundPayload{
			ProviderType: wakeupProviderType,
			Text:         message.TextMessage{Text: text},
			Actor:        actor,
		},
		PromptContext: &component.InboundPromptContext{Kind: "Wakeup"},
	})
}
