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
	v3component "github.com/bartdeboer/ctgbot/internal/v3/component"
)

const ComponentType = "telegram"

type Factory struct {
	Token   string
	Config  *appstate.Config
	Updates dbstorage.TelegramUpdateStorage
	Logger  *log.Logger
}

func NewFactory(token string, cfg *appstate.Config, updates dbstorage.TelegramUpdateStorage, logger *log.Logger) *Factory {
	return &Factory{
		Token:   strings.TrimSpace(token),
		Config:  cfg,
		Updates: updates,
		Logger:  logger,
	}
}

func (f *Factory) Type() string {
	return ComponentType
}

func (f *Factory) Create(ctx context.Context, req v3component.CreateRequest) (v3component.Component, error) {
	_ = ctx
	token := strings.TrimSpace(f.Token)
	if token == "" {
		return nil, fmt.Errorf("missing telegram token")
	}
	api, err := lowtelegram.NewTelegramAPIV2(token)
	if err != nil {
		return nil, err
	}
	return &Component{
		componentID: req.Registration.ID,
		bot:         lowtelegram.NewTelegramBot(api, f.Updates, f.Config, f.Logger),
	}, nil
}

type Component struct {
	componentID modeluuid.UUID
	bot         *lowtelegram.TelegramBot
}

func (c *Component) Type() string {
	return ComponentType
}

func (c *Component) RunInbound(ctx context.Context, emit v3component.InboundEmitter) error {
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
		return messenger.OutboundPayload{}, emit(cbCtx, v3component.InboundEvent{
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
