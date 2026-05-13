package broker

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

func noticeTextForDrop(ctx context.Context, app App, rejection *InboundRejection, drop *coremodel.DroppedEvent) string {
	if rejection == nil {
		return ""
	}
	text := strings.TrimSpace(rejection.NoticeText)
	if text == "" {
		return ""
	}
	dropID := ""
	if app != nil {
		dropID = app.DropNoticeID(ctx, drop)
	}
	return strings.ReplaceAll(text, "{{drop_id}}", dropID)
}

func (b *Broker) sendInboundRejectionNotice(ctx context.Context, rejection *InboundRejection, drop *coremodel.DroppedEvent) error {
	text := noticeTextForDrop(ctx, b.App, rejection, drop)
	if text == "" || rejection == nil || rejection.Chat == nil || rejection.SourceBinding == nil {
		return nil
	}
	thread, err := b.App.EnsureThread(ctx, *rejection.SourceBinding, strings.TrimSpace(rejection.Event.Payload.ProviderThreadID))
	if err != nil {
		return err
	}
	runtime, err := b.runtimeForChat(ctx, *rejection.Chat)
	if err != nil {
		return err
	}
	_, err = b.storeAndRelayMessage(ctx, runtime, *rejection.Chat, *thread, coremodel.ThreadMessage{
		Kind:        coremodel.MessageKindSystem,
		ComponentID: rejection.Event.ComponentID,
		ActorID:     "ctgbot",
		ActorLabel:  "ctgbot",
		Text:        text,
	}, "ctgbot")
	return err
}
