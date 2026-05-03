package telegram

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/dbstorage"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	lowtelegram "github.com/bartdeboer/ctgbot/internal/messenger/telegramengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
)

const Type = "telegram"

func New(ctx context.Context, registration coremodel.Component, profile component.Profile, runtime component.Runtime, home component.Home, storage repository.Storage, token string, cfg *appstate.Config, updates dbstorage.TelegramUpdateStorage, logger *log.Logger) (component.Component, error) {
	_, _, _, _, _ = ctx, profile, runtime, home, storage

	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("missing telegram token")
	}
	api, err := lowtelegram.NewTelegramAPIV2(token)
	if err != nil {
		return nil, err
	}
	return &Component{
		componentID: registration.ID,
		bot:         lowtelegram.NewTelegramBot(api, updates, cfg, logger),
	}, nil
}

type Component struct {
	componentID modeluuid.UUID
	bot         *lowtelegram.TelegramBot
}

func (c *Component) Type() string {
	return Type
}

func (c *Component) RunInbound(ctx context.Context, emit component.InboundEmitter) error {
	if c == nil || c.bot == nil {
		return fmt.Errorf("missing telegram bot")
	}
	if emit == nil {
		return fmt.Errorf("missing inbound emitter")
	}
	return c.bot.Run(ctx, func(cbCtx context.Context, payload messenger.InboundPayload) (messenger.OutboundPayload, error) {
		externalID := strings.TrimSpace(payload.ProviderMessageID)
		if externalID == "" {
			externalID = strings.TrimSpace(payload.ProviderChatID) + ":" + strings.TrimSpace(payload.ProviderThreadID)
		}
		return messenger.OutboundPayload{}, emit(cbCtx, component.InboundEvent{
			ComponentID: c.componentID,
			ExternalID:  externalID,
			Payload:     payload,
		})
	})
}

func (c *Component) Send(ctx context.Context, payload messenger.OutboundPayload) error {
	if c == nil || c.bot == nil {
		return fmt.Errorf("missing telegram bot")
	}
	return c.bot.Send(ctx, payload)
}

func (c *Component) StartChatAction(ctx context.Context, target messenger.ChatTarget, action messenger.ChatAction) (func(), error) {
	if c == nil || c.bot == nil {
		return nil, fmt.Errorf("missing telegram bot")
	}
	return c.bot.StartChatAction(ctx, target, action)
}
