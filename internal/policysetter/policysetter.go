package policysetter

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/configsetters"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type Elevation string

const (
	ElevationNone     Elevation = "none"
	ElevationChat     Elevation = "chat"
	ElevationElevated Elevation = "elevated"
)

type Context struct {
	ChatID    modeluuid.UUID
	UserID    int64
	IsAdmin   bool
	Elevation Elevation
}

type Setter struct {
	Name              string
	Help              string
	RequiredElevation Elevation

	Get func(ctx Context) (string, error)
	Set func(ctx Context, value string) (string, error)
}

func (s Setter) Allowed(ctx Context) bool {
	return elevationRank(ctx.Elevation) >= elevationRank(s.RequiredElevation)
}

func (s Setter) Visible(ctx Context) bool { return s.Allowed(ctx) }

func elevationRank(level Elevation) int {
	switch level {
	case ElevationElevated:
		return 2
	case ElevationChat:
		return 1
	default:
		return 0
	}
}

type Registry struct{ setters []Setter }

func NewRegistry(setters ...Setter) *Registry {
	out := make([]Setter, 0, len(setters))
	for _, setter := range setters {
		setter.Name = strings.TrimSpace(setter.Name)
		if setter.Name == "" {
			continue
		}
		out = append(out, setter)
	}
	return &Registry{setters: out}
}

func (r *Registry) List(ctx Context) []Setter {
	if r == nil {
		return nil
	}
	out := make([]Setter, 0, len(r.setters))
	for _, setter := range r.setters {
		if setter.Visible(ctx) {
			out = append(out, setter)
		}
	}
	return out
}

func (r *Registry) Find(name string) (Setter, bool) {
	if r == nil {
		return Setter{}, false
	}
	name = strings.TrimSpace(name)
	for _, setter := range r.setters {
		if setter.Name == name {
			return setter, true
		}
	}
	return Setter{}, false
}

func NewDefaultRegistry(cfg *configsetters.ConfigSetters) *Registry {
	return NewRegistry(
		SessionTimeoutSetter(cfg),
		PollTimeoutSetter(cfg),
		ChatProcessToolsSetter(cfg),
		ChatInteractiveInterruptSetter(cfg),
		ChatCodexProfileHostPathSetter(cfg),
		ChatEnabledSetter(cfg),
	)
}

func SessionTimeoutSetter(cfg *configsetters.ConfigSetters) Setter {
	setter := Setter{
		Name:              "session.timeout_min",
		Help:              "Session timeout duration",
		RequiredElevation: ElevationElevated,
	}
	setter.Get = func(Context) (string, error) {
		if cfg == nil || cfg.State == nil {
			return "", fmt.Errorf("missing app state")
		}
		return cfg.State.CodexSessionTimeout().String(), nil
	}
	setter.Set = func(ctx Context, value string) (string, error) {
		if !setter.Allowed(ctx) {
			return "", fmt.Errorf("setting %s requires %s chat access", setter.Name, setter.RequiredElevation)
		}
		value = strings.TrimSpace(value)
		if err := cfg.SetSessionTimeoutMin(configsetters.SetSessionTimeoutMinInput{SetSessionTimeoutMin: value}); err != nil {
			return "", err
		}
		return value, nil
	}
	return setter
}

func PollTimeoutSetter(cfg *configsetters.ConfigSetters) Setter {
	setter := Setter{
		Name:              "telegram.defaults.poll_timeout_sec",
		Help:              "Telegram poll timeout",
		RequiredElevation: ElevationElevated,
	}
	setter.Get = func(Context) (string, error) {
		if cfg == nil || cfg.State == nil {
			return "", fmt.Errorf("missing app state")
		}
		return cfg.State.TelegramPollTimeout().String(), nil
	}
	setter.Set = func(ctx Context, value string) (string, error) {
		if !setter.Allowed(ctx) {
			return "", fmt.Errorf("setting %s requires %s chat access", setter.Name, setter.RequiredElevation)
		}
		value = strings.TrimSpace(value)
		if err := cfg.SetPollTimeoutSec(configsetters.SetPollTimeoutSecInput{SetPollTimeoutSec: value}); err != nil {
			return "", err
		}
		return value, nil
	}
	return setter
}

func ChatProcessToolsSetter(cfg *configsetters.ConfigSetters) Setter {
	setter := Setter{
		Name:              "chat.process_tools_enabled",
		Help:              "Enable or disable process tools for the current chat",
		RequiredElevation: ElevationElevated,
	}
	setter.Get = func(ctx Context) (string, error) {
		if cfg == nil || cfg.State == nil {
			return "", fmt.Errorf("missing app state")
		}
		return strconv.FormatBool(cfg.State.ChatProcessToolsEnabledByID(ctx.ChatID)), nil
	}
	setter.Set = func(ctx Context, value string) (string, error) {
		if !setter.Allowed(ctx) {
			return "", fmt.Errorf("setting %s requires %s chat access", setter.Name, setter.RequiredElevation)
		}
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return "", fmt.Errorf("invalid bool %q", value)
		}
		if err := cfg.SetChatProcessToolsEnabled(configsetters.SetChatProcessToolsEnabledInput{ChatRoute: configsetters.ChatRoute{ChatID: ctx.ChatID.String()}, SetProcessToolsEnabled: parsed}); err != nil {
			return "", err
		}
		return strconv.FormatBool(parsed), nil
	}
	return setter
}

func ChatInteractiveInterruptSetter(cfg *configsetters.ConfigSetters) Setter {
	setter := Setter{
		Name:              "chat.interactive_interrupt_enabled",
		Help:              "Enable or disable interactive PTY-backed interrupt support for the current chat",
		RequiredElevation: ElevationElevated,
	}
	setter.Get = func(ctx Context) (string, error) {
		if cfg == nil || cfg.State == nil {
			return "", fmt.Errorf("missing app state")
		}
		return strconv.FormatBool(cfg.State.ChatInteractiveInterruptEnabledByID(ctx.ChatID)), nil
	}
	setter.Set = func(ctx Context, value string) (string, error) {
		if !setter.Allowed(ctx) {
			return "", fmt.Errorf("setting %s requires %s chat access", setter.Name, setter.RequiredElevation)
		}
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return "", fmt.Errorf("invalid bool %q", value)
		}
		if err := cfg.SetChatInteractiveInterruptEnabled(configsetters.SetChatInteractiveInterruptEnabledInput{ChatRoute: configsetters.ChatRoute{ChatID: ctx.ChatID.String()}, SetInteractiveInterruptEnabled: parsed}); err != nil {
			return "", err
		}
		return strconv.FormatBool(parsed), nil
	}
	return setter
}

func ChatEnabledSetter(cfg *configsetters.ConfigSetters) Setter {
	setter := Setter{
		Name:              "chat.enabled",
		Help:              "Enable or disable the current chat",
		RequiredElevation: ElevationElevated,
	}
	setter.Get = func(ctx Context) (string, error) {
		if cfg == nil || cfg.State == nil {
			return "", fmt.Errorf("missing app state")
		}
		return strconv.FormatBool(cfg.State.ChatEnabledByID(ctx.ChatID)), nil
	}
	setter.Set = func(ctx Context, value string) (string, error) {
		if !setter.Allowed(ctx) {
			return "", fmt.Errorf("setting %s requires %s chat access", setter.Name, setter.RequiredElevation)
		}
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err != nil {
			return "", fmt.Errorf("invalid bool %q", value)
		}
		if err := cfg.SetChatEnabled(configsetters.SetChatEnabledInput{ChatRoute: configsetters.ChatRoute{ChatID: ctx.ChatID.String()}, SetEnabled: parsed}); err != nil {
			return "", err
		}
		return strconv.FormatBool(parsed), nil
	}
	return setter
}

func ChatCodexProfileHostPathSetter(cfg *configsetters.ConfigSetters) Setter {
	setter := Setter{
		Name:              "chat.codex_profile_host_path",
		Help:              "Override the Codex profile host path for the current chat",
		RequiredElevation: ElevationElevated,
	}
	setter.Get = func(ctx Context) (string, error) {
		if cfg == nil || cfg.State == nil {
			return "", fmt.Errorf("missing app state")
		}
		return cfg.State.ChatCodexProfileHostPathByID(ctx.ChatID), nil
	}
	setter.Set = func(ctx Context, value string) (string, error) {
		if !setter.Allowed(ctx) {
			return "", fmt.Errorf("setting %s requires %s chat access", setter.Name, setter.RequiredElevation)
		}
		value = strings.TrimSpace(value)
		if err := cfg.SetChatCodexProfileHostPath(configsetters.SetChatCodexProfileHostPathInput{ChatRoute: configsetters.ChatRoute{ChatID: ctx.ChatID.String()}, SetCodexProfileHostPath: value}); err != nil {
			return "", err
		}
		return cfg.State.ChatCodexProfileHostPathByID(ctx.ChatID), nil
	}
	return setter
}
