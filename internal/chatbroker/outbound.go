package chatbroker

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/messenger"
)

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

	return provider.SendFile(ctx, messenger.ResolvedOutgoingFile{
		ProviderChatID:   strings.TrimSpace(chatCfg.ProviderChatID),
		ProviderThreadID: strings.TrimSpace(thread.ProviderThreadID),
		Filename:         strings.TrimSpace(file.Filename),
		Caption:          strings.TrimSpace(file.Caption),
		Content:          append([]byte(nil), file.Content...),
	})
}
