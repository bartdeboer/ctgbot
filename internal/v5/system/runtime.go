package system

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
)

func (s *System) ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*component.Loaded, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing system storage")
	}
	if s.Registry == nil {
		return nil, fmt.Errorf("missing component registry")
	}
	cacheKey := componentID.String()
	if loaded := s.loaded[cacheKey]; loaded != nil {
		return loaded, nil
	}

	registration, err := s.Storage.Components().GetByID(ctx, componentID)
	if err != nil {
		return nil, err
	}
	if registration == nil {
		return nil, fmt.Errorf("component not found: %s", componentID)
	}

	profile, err := s.Profile(registration.Profile)
	if err != nil {
		return nil, err
	}
	runtime, err := s.Runtime(profile.Name)
	if err != nil {
		return nil, err
	}
	home := runtime.ComponentHome(*registration)
	if err := os.MkdirAll(home.HostPath, 0o755); err != nil {
		return nil, err
	}

	loaded, err := s.Registry.Build(ctx, *registration, runtime, home, s.Storage)
	if err != nil {
		return nil, err
	}
	if s.loaded == nil {
		s.loaded = map[string]*component.Loaded{}
	}
	s.loaded[cacheKey] = loaded
	return loaded, nil
}

func (s *System) Profile(name string) (v5runtime.Profile, error) {
	if s == nil {
		return v5runtime.Profile{}, fmt.Errorf("missing system")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = "default"
	}
	profile, ok := s.Profiles[name]
	if !ok {
		return v5runtime.Profile{}, fmt.Errorf("profile not found: %s", name)
	}
	return profile, nil
}

func (s *System) Runtime(profileName string) (v5runtime.Factory, error) {
	if s == nil {
		return nil, fmt.Errorf("missing system")
	}
	profileName = strings.TrimSpace(profileName)
	if profileName == "" {
		profileName = "default"
	}
	factory, ok := s.Runtimes[profileName]
	if !ok {
		return nil, fmt.Errorf("runtime not found for profile: %s", profileName)
	}
	return factory, nil
}

func (s *System) EnsureComponent(ctx context.Context, ref string, profileName string) (*coremodel.Component, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing system storage")
	}
	if s.Registry == nil {
		return nil, fmt.Errorf("missing component registry")
	}

	parsed, err := coremodel.ParseComponentRef(ref)
	if err != nil {
		return nil, err
	}
	if !s.Registry.Has(parsed.Type) {
		return nil, fmt.Errorf("component type not registered in code: %s", parsed.Type)
	}

	registration, err := s.Storage.Components().GetByTypeAndName(ctx, parsed.Type, parsed.ResolvedName())
	if err != nil {
		return nil, err
	}
	if registration == nil {
		if strings.TrimSpace(profileName) == "" {
			profileName = "default"
		}
		profile, err := s.Profile(profileName)
		if err != nil {
			return nil, err
		}
		registration = &coremodel.Component{
			Type:      parsed.Type,
			Name:      parsed.ResolvedName(),
			Profile:   profile.Name,
			Enabled:   true,
			IsDefault: !parsed.ExplicitName || parsed.ResolvedName() == coremodel.DefaultComponentName(parsed.Type),
		}
	} else {
		registration.Enabled = true
		if strings.TrimSpace(profileName) != "" {
			profile, err := s.Profile(profileName)
			if err != nil {
				return nil, err
			}
			registration.Profile = profile.Name
		} else if strings.TrimSpace(registration.Profile) == "" {
			registration.Profile = "default"
		}
	}

	if err := s.Storage.Components().Save(ctx, registration); err != nil {
		return nil, err
	}
	runtime, err := s.Runtime(registration.Profile)
	if err != nil {
		return nil, err
	}
	home := runtime.ComponentHome(*registration)
	if err := os.MkdirAll(home.HostPath, 0o755); err != nil {
		return nil, err
	}
	delete(s.loaded, registration.ID.String())
	return registration, nil
}

func (s *System) BindChatComponent(ctx context.Context, chatID modeluuid.UUID, role coremodel.ChatComponentRole, ref string, externalChatID string) (*coremodel.ChatComponent, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing system storage")
	}
	if chatID.IsNull() {
		return nil, fmt.Errorf("missing chat id")
	}
	if !role.Valid() {
		return nil, fmt.Errorf("invalid chat component role: %q", role)
	}

	chat, err := s.Storage.Chats().GetByID(ctx, chatID)
	if err != nil {
		return nil, err
	}
	if chat == nil {
		return nil, fmt.Errorf("chat not found: %s", chatID)
	}

	registration, err := s.ResolveComponentRef(ctx, ref)
	if err != nil {
		return nil, err
	}

	externalChatID = strings.TrimSpace(externalChatID)
	switch role {
	case coremodel.ChatComponentRoleSource, coremodel.ChatComponentRoleRelay:
		if externalChatID == "" {
			return nil, fmt.Errorf("missing external chat id for role %q", role)
		}
	default:
		externalChatID = ""
	}

	binding, err := s.Storage.ChatComponents().GetByChatComponentRole(ctx, chatID, registration.ID, role)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		binding = &coremodel.ChatComponent{
			ChatID:      chatID,
			ComponentID: registration.ID,
			Role:        role,
		}
	}
	binding.ExternalChatID = externalChatID
	binding.Enabled = true
	if err := s.Storage.ChatComponents().Save(ctx, binding); err != nil {
		return nil, err
	}
	return binding, nil
}

func (s *System) ResolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing system storage")
	}
	parsed, err := coremodel.ParseComponentRef(ref)
	if err != nil {
		return nil, err
	}
	if !parsed.ExplicitName {
		registration, err := s.Storage.Components().GetDefaultByType(ctx, parsed.Type)
		if err != nil {
			return nil, err
		}
		if registration != nil {
			return registration, nil
		}
	}
	registration, err := s.Storage.Components().GetByTypeAndName(ctx, parsed.Type, parsed.ResolvedName())
	if err != nil {
		return nil, err
	}
	if registration == nil {
		return nil, fmt.Errorf("component not registered: %s", parsed.Ref())
	}
	return registration, nil
}
