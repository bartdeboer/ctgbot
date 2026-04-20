package configcommands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/policysetter"
)

type Service struct {
	Registry *policysetter.Registry
}

func New(registry *policysetter.Registry) *Service {
	return &Service{Registry: registry}
}

func ContextForChat(cfg *appstate.Config, chatID modeluuid.UUID, userID int64, isAdmin bool) policysetter.Context {
	elevation := policysetter.ElevationNone
	if cfg != nil && cfg.ChatEnabledByID(chatID) {
		elevation = policysetter.ElevationChat
		if cfg.ChatProcessToolsEnabledByID(chatID) {
			elevation = policysetter.ElevationElevated
		}
	}
	return policysetter.Context{ChatID: chatID, UserID: userID, IsAdmin: isAdmin, Elevation: elevation}
}

func (s *Service) List(ctx policysetter.Context) (string, error) {
	if s == nil || s.Registry == nil {
		return "no settings available", nil
	}
	setters := s.Registry.List(ctx)
	if len(setters) == 0 {
		return "no settings available", nil
	}
	sort.Slice(setters, func(i, j int) bool { return setters[i].Name < setters[j].Name })
	lines := make([]string, 0, len(setters))
	for _, setter := range setters {
		value, err := setter.Get(ctx)
		if err != nil {
			return "", err
		}
		line := fmt.Sprintf("%s = %s", setter.Name, value)
		if setter.RequiredElevation != "" {
			line += fmt.Sprintf(" (%s)", setter.RequiredElevation)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n"), nil
}

func (s *Service) Set(ctx policysetter.Context, key, value string) (string, error) {
	if s == nil || s.Registry == nil {
		return "", fmt.Errorf("config commands are unavailable")
	}
	setter, ok := s.Registry.Find(strings.TrimSpace(key))
	if !ok {
		return "", fmt.Errorf("unknown setting: %s", strings.TrimSpace(key))
	}
	if !setter.Allowed(ctx) {
		return "", fmt.Errorf("setting %s is not allowed in this context", setter.Name)
	}
	applied, err := setter.Set(ctx, value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("set %s = %s", setter.Name, applied), nil
}
