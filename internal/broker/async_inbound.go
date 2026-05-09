package broker

import (
	"context"
	"fmt"

	component "github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/message"
)

func (b *Broker) HandleResolvedInboundAsync(ctx context.Context, inbound component.ResolvedInbound) error {
	if err := b.ensureReady(); err != nil {
		return err
	}
	if inbound.Chat.ID.IsNull() {
		return fmt.Errorf("missing inbound chat id")
	}
	if inbound.Thread.ID.IsNull() {
		return fmt.Errorf("missing inbound thread id")
	}

	queued := cloneResolvedInbound(inbound)
	runCtx := context.Background()
	if ctx != nil {
		runCtx = context.WithoutCancel(ctx)
	}
	go func() {
		if _, err := b.HandleResolvedInbound(runCtx, queued); err != nil {
			b.logf("async inbound failed chat=%s thread=%s err=%v", queued.Chat.ID, queued.Thread.ID, err)
		}
	}()
	return nil
}

func cloneResolvedInbound(inbound component.ResolvedInbound) component.ResolvedInbound {
	out := inbound
	out.Metadata = append([]string(nil), inbound.Metadata...)
	if inbound.PromptContext != nil {
		copyPrompt := *inbound.PromptContext
		out.PromptContext = &copyPrompt
	}
	out.Payload.Actor.Roles = append(out.Payload.Actor.Roles[:0:0], inbound.Payload.Actor.Roles...)
	if len(inbound.Payload.Attachments) > 0 {
		out.Payload.Attachments = make([]message.Media, len(inbound.Payload.Attachments))
		for i, media := range inbound.Payload.Attachments {
			out.Payload.Attachments[i] = media
			out.Payload.Attachments[i].Content = append([]byte(nil), media.Content...)
		}
	}
	return out
}
