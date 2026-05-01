package broker

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

func (b *Broker) appendAndRelayOutbound(ctx context.Context, message *coremodel.ThreadMessage, inbound coremodel.ThreadMessage, runtime *ChatRuntime) error {
	message.ChatID = inbound.ChatID
	message.ThreadID = inbound.ThreadID
	message.Direction = coremodel.DirectionOutbound
	if message.Kind == "" {
		message.Kind = coremodel.MessageKindAgent
	}
	if message.MetadataJSON == "" {
		message.MetadataJSON = inbound.MetadataJSON
	}
	if err := b.storage.Messages().Append(ctx, message); err != nil {
		return err
	}
	b.logf("v2 outbound stored message=%s source=%s chars=%d", message.ID, message.SourceType, len(message.Text))
	return b.relayOutbound(ctx, *message, runtime)
}

func (b *Broker) relayOutbound(ctx context.Context, message coremodel.ThreadMessage, runtime *ChatRuntime) error {
	if runtime == nil || len(runtime.Relays) == 0 {
		return nil
	}
	for _, relay := range runtime.Relays {
		b.logf("v2 relay sending type=%s message=%s", relay.Type(), message.ID)
		if err := relay.SendMessage(ctx, message); err != nil {
			return fmt.Errorf("relay %s: %w", relay.Type(), err)
		}
		b.logf("v2 relay sent type=%s message=%s", relay.Type(), message.ID)
	}
	return nil
}
