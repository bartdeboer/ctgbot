package chatbroker

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/messenger"
)

func (b *Broker) SendText(ctx context.Context, msg messenger.OutgoingMessage) error {
	if b == nil || b.Config == nil {
		return fmt.Errorf("missing config")
	}
	if b.Sessions == nil {
		return fmt.Errorf("missing session store")
	}
	if msg.SandboxID.IsNull() {
		return fmt.Errorf("sandbox id is null")
	}
	if strings.TrimSpace(msg.Text) == "" {
		return fmt.Errorf("missing text")
	}

	thread, err := b.Sessions.FindThreadByID(ctx, msg.SandboxID)
	if err != nil {
		return fmt.Errorf("find thread: %w", err)
	}
	if thread == nil {
		return fmt.Errorf("thread not found: %s", msg.SandboxID)
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

	return provider.SendText(ctx, messenger.ResolvedOutgoingMessage{
		ProviderChatID:   strings.TrimSpace(chatCfg.ProviderChatID),
		ProviderThreadID: strings.TrimSpace(thread.ProviderThreadID),
		Text:             msg.Text,
	})
}

func (b *Broker) SendFile(ctx context.Context, file messenger.OutgoingFile) error {
	if b == nil || b.Config == nil {
		return fmt.Errorf("missing config")
	}
	if b.Sessions == nil {
		return fmt.Errorf("missing session store")
	}
	if file.SandboxID.IsNull() {
		return fmt.Errorf("sandbox id is null")
	}
	if strings.TrimSpace(file.Filename) == "" {
		return fmt.Errorf("missing filename")
	}

	thread, err := b.Sessions.FindThreadByID(ctx, file.SandboxID)
	if err != nil {
		return fmt.Errorf("find thread: %w", err)
	}
	if thread == nil {
		return fmt.Errorf("thread not found: %s", file.SandboxID)
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

	return provider.SendFile(ctx, messenger.ResolvedOutgoingFile{
		ProviderChatID:   strings.TrimSpace(chatCfg.ProviderChatID),
		ProviderThreadID: strings.TrimSpace(thread.ProviderThreadID),
		Filename:         strings.TrimSpace(file.Filename),
		Caption:          strings.TrimSpace(file.Caption),
		Content:          append([]byte(nil), file.Content...),
	})
}
