package chatbroker

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (b *Broker) SendPayload(ctx context.Context, sandboxID modeluuid.UUID, payload messenger.OutboundPayload) error {
	if b == nil || b.Config == nil {
		return fmt.Errorf("missing config")
	}
	if b.Sessions == nil {
		return fmt.Errorf("missing session store")
	}
	if sandboxID.IsNull() {
		return fmt.Errorf("sandbox id is null")
	}

	thread, err := b.Sessions.FindThreadByID(ctx, sandboxID)
	if err != nil {
		return fmt.Errorf("find thread: %w", err)
	}
	if thread == nil {
		return fmt.Errorf("thread not found: %s", sandboxID)
	}

	chatCfg, err := b.Config.FindChatByID(thread.ChatID)
	if err != nil {
		return fmt.Errorf("find chat: %w", err)
	}
	if chatCfg == nil {
		return fmt.Errorf("chat not found: %s", thread.ChatID)
	}

	provider, ok := b.OutboundProviders[chatCfg.ProviderType]
	if !ok || provider == nil {
		return fmt.Errorf("outbound provider not registered: %s", chatCfg.ProviderType)
	}

	stopUpload, err := provider.StartChatAction(ctx, messenger.ChatTarget{
		ProviderChatID:   strings.TrimSpace(chatCfg.ProviderChatID),
		ProviderThreadID: strings.TrimSpace(thread.ProviderThreadID),
	}, messenger.ChatActionUploadDocument)
	if err != nil {
		b.logf("start file upload chat action failed chat=%s thread=%s err=%v", chatCfg.ID, thread.ID, err)
		stopUpload = func() {}
	}
	if stopUpload == nil {
		stopUpload = func() {}
	}
	defer stopUpload()

	resolved := payload
	resolved.ProviderChatID = strings.TrimSpace(chatCfg.ProviderChatID)
	resolved.ProviderThreadID = strings.TrimSpace(thread.ProviderThreadID)
	resolved.Text.Text = strings.TrimSpace(resolved.Text.Text)
	resolved.Attachments = append([]messenger.Media(nil), resolved.Attachments...)
	for i := range resolved.Attachments {
		resolved.Attachments[i].Filename = strings.TrimSpace(resolved.Attachments[i].Filename)
		resolved.Attachments[i].ContentType = strings.TrimSpace(resolved.Attachments[i].ContentType)
		resolved.Attachments[i].Syntax = strings.TrimSpace(resolved.Attachments[i].Syntax)
		resolved.Attachments[i].Content = append([]byte(nil), resolved.Attachments[i].Content...)
	}
	return provider.Send(ctx, resolved)
}
