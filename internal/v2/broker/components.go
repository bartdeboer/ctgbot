package broker

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

func (b *Broker) ensureSourceChatComponent(ctx context.Context, chatID modeluuid.UUID, event component.InboundEvent) error {
	componentType := strings.TrimSpace(event.SourceType)
	if componentType == "" {
		return nil
	}
	profileName := strings.TrimSpace(event.SourceProfileName)
	existing, err := b.storage.ChatComponents().ListByChatID(ctx, chatID)
	if err != nil {
		return err
	}
	sourceKey := component.Key{Type: componentType, Name: profileName}
	for _, binding := range existing {
		if component.KeyForBinding(binding).Fingerprint() == sourceKey.Fingerprint() {
			return nil
		}
	}
	return b.storage.ChatComponents().Save(ctx, &coremodel.ChatComponent{
		ChatID:        chatID,
		ComponentType: componentType,
		ProfileName:   profileName,
		Enabled:       true,
	})
}

func (b *Broker) enabledChatComponents(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ChatComponent, error) {
	return b.storage.ChatComponents().ListEnabledByChatID(ctx, chatID)
}
