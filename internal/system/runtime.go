package system

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

func (s *System) ResolveComponent(ctx context.Context, componentID modeluuid.UUID) (*component.Loaded, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing system storage")
	}
	if s.Registry == nil {
		return nil, fmt.Errorf("missing component registry")
	}
	cacheKey := componentID.String()
	s.loadedMu.RLock()
	loaded := s.loaded[cacheKey]
	s.loadedMu.RUnlock()
	if loaded != nil {
		return loaded, nil
	}

	registration, err := s.Storage.Components().GetByID(ctx, componentID)
	if err != nil {
		return nil, err
	}
	if registration == nil {
		return nil, fmt.Errorf("component not found: %s", componentID)
	}

	runtime, err := s.Runtime(registration.Runtime)
	if err != nil {
		return nil, err
	}
	profile := runtime.ComponentProfile(*registration)
	if err := os.MkdirAll(profile.Path, 0o755); err != nil {
		return nil, err
	}

	loaded, err = s.Registry.Build(ctx, *registration, runtime, profile, s.Storage)
	if err != nil {
		return nil, err
	}
	s.loadedMu.Lock()
	defer s.loadedMu.Unlock()
	if s.loaded == nil {
		s.loaded = map[string]*component.Loaded{}
	}
	s.loaded[cacheKey] = loaded
	return loaded, nil
}

func (s *System) Workspace(name string) (Workspace, error) {
	if s == nil {
		return Workspace{}, fmt.Errorf("missing system")
	}
	name = strings.TrimSpace(name)
	workspace, ok := s.Workspaces[name]
	if !ok {
		return Workspace{}, fmt.Errorf("workspace not found: %s", name)
	}
	return workspace, nil
}

func (s *System) ValidateWorkspace(name string) error {
	_, err := s.Workspace(name)
	return err
}

func (s *System) Runtime(runtimeKind string) (runtimepkg.Factory, error) {
	if s == nil {
		return nil, fmt.Errorf("missing system")
	}
	runtimeKind = strings.TrimSpace(runtimeKind)
	if runtimeKind == "" {
		runtimeKind = "docker"
	}
	factory, ok := s.Runtimes[runtimeKind]
	if !ok {
		return nil, fmt.Errorf("runtime not found: %s", runtimeKind)
	}
	return factory, nil
}

func (s *System) ResolveChatWorkspace(_ context.Context, chat coremodel.Chat) (string, error) {
	if s == nil {
		return "", fmt.Errorf("missing system")
	}
	workspaceName := strings.TrimSpace(chat.Workspace)
	if workspaceName != "" {
		workspace, err := s.Workspace(workspaceName)
		if err != nil {
			return "", err
		}
		hostPath := workspace.Path
		if err := os.MkdirAll(filepath.Join(hostPath, "inbox"), 0o755); err != nil {
			return "", err
		}
		return hostPath, nil
	}
	if chat.ID.IsNull() {
		return "", fmt.Errorf("missing chat id")
	}
	hostPath := filepath.Join(s.StateRoot, "chats", chat.ID.String(), "workspace")
	if err := os.MkdirAll(filepath.Join(hostPath, "inbox"), 0o755); err != nil {
		return "", err
	}
	return hostPath, nil
}

func (s *System) ResolveChatHostbridgeAliases(_ context.Context, chat coremodel.Chat) (map[string]hostbridgeserver.Alias, error) {
	if s == nil {
		return nil, fmt.Errorf("missing system")
	}
	workspaceName := strings.TrimSpace(chat.Workspace)
	if workspaceName == "" {
		return nil, nil
	}
	workspace, err := s.Workspace(workspaceName)
	if err != nil {
		return nil, err
	}
	if len(workspace.HostbridgeAliases) == 0 {
		return nil, nil
	}
	out := make(map[string]hostbridgeserver.Alias, len(workspace.HostbridgeAliases))
	for name, spec := range workspace.HostbridgeAliases {
		out[name] = spec
	}
	return out, nil
}

func (s *System) ThreadExtraInstructions(_ context.Context, threadID modeluuid.UUID) (string, error) {
	if s == nil || s.StateRoot == "" || threadID.IsNull() {
		return "", nil
	}
	path := filepath.Join(s.StateRoot, "threads", threadID.String(), "extra-instructions.md")
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(body)), nil
}

func (s *System) EnsureComponent(ctx context.Context, ref string, runtimeKind string, profilePath string) (*coremodel.Component, error) {
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
		runtimeKind = strings.TrimSpace(runtimeKind)
		if runtimeKind == "" {
			runtimeKind = "docker"
		}
		runtime, err := s.Runtime(runtimeKind)
		if err != nil {
			return nil, err
		}
		registration = &coremodel.Component{
			Type:        parsed.Type,
			Name:        parsed.ResolvedName(),
			Runtime:     runtime.Kind(),
			ProfilePath: strings.TrimSpace(profilePath),
			Enabled:     true,
			IsDefault:   !parsed.ExplicitName || parsed.ResolvedName() == coremodel.DefaultComponentName(parsed.Type),
		}
	} else {
		registration.Enabled = true
		if strings.TrimSpace(runtimeKind) != "" {
			runtime, err := s.Runtime(runtimeKind)
			if err != nil {
				return nil, err
			}
			registration.Runtime = runtime.Kind()
		} else if strings.TrimSpace(registration.Runtime) == "" {
			registration.Runtime = "docker"
		}
		if strings.TrimSpace(profilePath) != "" {
			registration.ProfilePath = strings.TrimSpace(profilePath)
		}
	}

	if err := s.Storage.Components().Save(ctx, registration); err != nil {
		return nil, err
	}
	runtime, err := s.Runtime(registration.Runtime)
	if err != nil {
		return nil, err
	}
	profile := runtime.ComponentProfile(*registration)
	if err := os.MkdirAll(profile.Path, 0o755); err != nil {
		return nil, err
	}
	s.loadedMu.Lock()
	delete(s.loaded, registration.ID.String())
	s.loadedMu.Unlock()
	return registration, nil
}

func (s *System) BindChatComponent(ctx context.Context, chatID modeluuid.UUID, role coremodel.ChatComponentRole, ref string, externalChannelID string) (*coremodel.ChatComponent, error) {
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

	externalChannelID = strings.TrimSpace(externalChannelID)
	switch role {
	case coremodel.ChatComponentRoleSource, coremodel.ChatComponentRoleRelay:
		if externalChannelID == "" {
			return nil, fmt.Errorf("missing external channel id for role %q", role)
		}
	default:
		externalChannelID = ""
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
	binding.ExternalChannelID = externalChannelID
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
