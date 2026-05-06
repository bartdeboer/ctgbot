package telegram2

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
)

const Type = "telegram"

func New(
	ctx context.Context,
	registration coremodel.Component,
	runtime v5runtime.Factory,
	home v5runtime.Home,
	storage repository.Storage,
	token string,
	cfg *appstate.Config,
	logger *log.Logger,
) (component.Component, error) {
	_, _, _, _ = ctx, runtime, home, storage

	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("missing telegram token")
	}
	api, err := NewTelegramAPIV2(token)
	if err != nil {
		return nil, err
	}
	return &Component{
		componentID: registration.ID,
		api:         api,
		cfg:         cfg,
		logger:      logger,
	}, nil
}

type Component struct {
	componentID modeluuid.UUID
	api         TelegramAPI
	cfg         *appstate.Config
	logger      *log.Logger
}

func (c *Component) Type() string {
	return Type
}

func (c *Component) logf(format string, args ...any) {
	if c != nil && c.logger != nil {
		c.logger.Printf(format, args...)
	}
}
