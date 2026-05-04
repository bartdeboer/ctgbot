package broker

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (b *Broker) SendPayload(
	ctx context.Context,
	threadID modeluuid.UUID,
	payload messenger.OutboundPayload,
) error {
	if err := b.ensureReady(); err != nil {
		return err
	}
	if threadID.IsNull() {
		return fmt.Errorf("missing thread id")
	}

	thread, err := b.Storage.Threads().GetByID(ctx, threadID)
	if err != nil {
		return err
	}
	if thread == nil {
		return fmt.Errorf("thread not found: %s", threadID)
	}

	chat, err := b.Storage.Chats().GetByID(ctx, thread.ChatID)
	if err != nil {
		return err
	}
	if chat == nil {
		return fmt.Errorf("chat not found: %s", thread.ChatID)
	}

	runtime, err := b.runtimeForChat(ctx, *chat)
	if err != nil {
		return err
	}
	_, err = b.deliverPayload(ctx, runtime, *chat, *thread, payload, modeluuid.UUID{})
	return err
}
