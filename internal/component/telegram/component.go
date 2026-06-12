package telegram

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

const Type = "telegram"

var errMissingTelegramToken = errors.New("missing telegram token")

func New(
	ctx context.Context,
	registration coremodel.Component,
	runtime runtimepkg.Factory,
	profile runtimepkg.Profile,
	storage repository.Storage,
	logger *log.Logger,
) (component.Component, error) {
	_, _, _ = ctx, runtime, storage

	config, err := loadComponentConfig(profile.Path)
	if err != nil {
		return nil, err
	}
	c := &Component{
		componentID:     registration.ID,
		profile:         profile,
		componentConfig: config,
		logger:          logger,
	}
	if err := c.loadAPIFromProfile(); err != nil && !errors.Is(err, errMissingTelegramToken) {
		return nil, err
	}
	return c, nil
}

type Component struct {
	componentID     modeluuid.UUID
	profile         runtimepkg.Profile
	componentConfig ComponentConfig
	api             TelegramAPI
	logger          *log.Logger
}

var _ component.Component = (*Component)(nil)
var _ component.InboundSource = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)
var _ component.OutboundRelay = (*Component)(nil)

func (c *Component) Type() string {
	return Type
}

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: TokenFilename, Required: true, Sensitive: true},
		{RelativePath: ComponentConfigFilename, Required: false, Sensitive: false},
	}
}

func (c *Component) loadAPIFromProfile() error {
	if c == nil {
		return fmt.Errorf("missing telegram component")
	}
	token, err := loadToken(c.profile.Path)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errMissingTelegramToken
	}
	api, err := NewTelegramAPIV2(token)
	if err != nil {
		return err
	}
	c.api = api
	return nil
}

func (c *Component) logf(format string, args ...any) {
	if c != nil && c.logger != nil {
		c.logger.Printf(format, args...)
	}
}
