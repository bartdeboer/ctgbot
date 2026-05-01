package broker

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

func (b *Broker) ensureDefaultChatComponents(ctx context.Context, chatID modeluuid.UUID) error {
	existing, err := b.storage.ChatComponents().ListByChatID(ctx, chatID)
	if err != nil {
		return err
	}
	seen := map[string]struct{}{}
	for _, binding := range existing {
		seen[chatComponentKey(binding.ComponentType, binding.ProfileName)] = struct{}{}
	}

	for _, binding := range b.defaultChatComponents {
		if strings.TrimSpace(binding.ComponentType) == "" {
			continue
		}
		key := chatComponentKey(binding.ComponentType, binding.ProfileName)
		if _, ok := seen[key]; ok {
			continue
		}
		binding.ChatID = chatID
		if err := b.storage.ChatComponents().Save(ctx, &binding); err != nil {
			return err
		}
		seen[key] = struct{}{}
	}
	return nil
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
