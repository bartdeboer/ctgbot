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
	for _, binding := range existing {
		if chatComponentKey(binding.ComponentType, binding.ProfileName) == chatComponentKey(componentType, profileName) {
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

func chatComponentKey(componentType string, profileName string) string {
	return strings.TrimSpace(componentType) + "\x00" + strings.TrimSpace(profileName)
}

func (b *Broker) enabledChatComponents(ctx context.Context, chatID modeluuid.UUID) ([]coremodel.ChatComponent, error) {
	return b.storage.ChatComponents().ListEnabledByChatID(ctx, chatID)
}

func matchesAnyBinding(candidate component.Component, bindings []coremodel.ChatComponent) bool {
	for _, binding := range bindings {
		if component.MatchesBinding(candidate, binding) {
			return true
		}
	}
	return false
}
