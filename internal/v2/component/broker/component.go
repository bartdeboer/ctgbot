// Package broker exposes ctgbot broker administration as a v2 command surface.
package broker

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
	v2codex "github.com/bartdeboer/ctgbot/internal/v2/component/codex"
	v2runtimecomponent "github.com/bartdeboer/ctgbot/internal/v2/component/runtime"
	v2telegram "github.com/bartdeboer/ctgbot/internal/v2/component/telegram"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v2/repository"
	v2commands "github.com/bartdeboer/ctgbot/internal/v2/schema/commands"
	v2routers "github.com/bartdeboer/ctgbot/internal/v2/schema/routers"
)

const (
	ComponentType          = "broker"
	PresetTelegramCodex    = "telegram-codex"
	DefaultRuntimeProfile  = ""
	unregisteredListHeader = "unregistered chats"
)

type Config struct {
	CodexProfile string
}

type Component struct {
	Storage repository.Storage
	Config  Config
}

var _ component.Component = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ v2routers.ChatHandlers = (*Component)(nil)

func New(storage repository.Storage, cfg Config) *Component {
	return &Component{Storage: storage, Config: cfg}
}

func (c *Component) Type() string {
	return ComponentType
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return v2commands.ChatCommands()
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	return v2routers.RegisterChatHandlers(registry, c)
}

func (c *Component) ListUnregisteredChats(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	storage, err := c.storage()
	if err != nil {
		return commandengine.Result{}, err
	}
	chats, err := storage.Chats().ListDisabled(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(chats) == 0 {
		return commandengine.Result{Text: "no unregistered chats"}, nil
	}

	var b strings.Builder
	b.WriteString(unregisteredListHeader)
	for _, chat := range chats {
		fmt.Fprintf(&b, "\n%s %s:%s", chat.ID, chat.ProviderType, chat.ProviderChatID)
	}
	return commandengine.Result{Text: b.String()}, nil
}

func (c *Component) ApplyChatPreset(ctx context.Context, req commandengine.Request, cmd v2commands.ChatApplyPreset) (commandengine.Result, error) {
	if strings.TrimSpace(cmd.Preset) != PresetTelegramCodex {
		return commandengine.Result{}, fmt.Errorf("unsupported chat preset %q", strings.TrimSpace(cmd.Preset))
	}
	storage, err := c.storage()
	if err != nil {
		return commandengine.Result{}, err
	}
	chatID, err := modeluuid.Parse(strings.TrimSpace(cmd.ChatID))
	if err != nil {
		return commandengine.Result{}, fmt.Errorf("invalid chat id %q", strings.TrimSpace(cmd.ChatID))
	}
	chat, err := storage.Chats().GetByID(ctx, chatID)
	if err != nil {
		return commandengine.Result{}, err
	}
	if chat == nil {
		return commandengine.Result{}, fmt.Errorf("chat not found: %s", chatID)
	}
	codexProfile := strings.TrimSpace(c.Config.CodexProfile)
	if codexProfile == "" {
		return commandengine.Result{}, fmt.Errorf("missing codex profile")
	}
	chat.Enabled = true
	if err := storage.Chats().Save(ctx, chat); err != nil {
		return commandengine.Result{}, err
	}

	for _, binding := range []coremodel.ChatComponent{
		{ChatID: chat.ID, ComponentType: v2telegram.ComponentType, ProfileName: v2telegram.DefaultProfileName, Enabled: true},
		{ChatID: chat.ID, ComponentType: v2codex.ComponentType, ProfileName: codexProfile, Enabled: true},
		{ChatID: chat.ID, ComponentType: v2runtimecomponent.ComponentType, ProfileName: DefaultRuntimeProfile, Enabled: true},
	} {
		if err := storage.ChatComponents().Save(ctx, &binding); err != nil {
			return commandengine.Result{}, err
		}
	}
	return commandengine.Result{Text: fmt.Sprintf("chat %s enabled with preset %s", chat.ID, PresetTelegramCodex)}, nil
}

func (c *Component) storage() (repository.Storage, error) {
	if c == nil || c.Storage == nil {
		return nil, fmt.Errorf("missing broker command storage")
	}
	return c.Storage, nil
}
