package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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
	home := runtime.ComponentHome(*registration)
	if err := os.MkdirAll(home.Path, 0o755); err != nil {
		return nil, err
	}

	loaded, err = s.Registry.Build(ctx, *registration, runtime, home, s.Storage)
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

func (s *System) Runtime(runtimeKind string) (v5runtime.Factory, error) {
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

func (s *System) EnsureComponent(ctx context.Context, ref string, runtimeKind string, homePath string) (*coremodel.Component, error) {
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
			Type:      parsed.Type,
			Name:      parsed.ResolvedName(),
			Runtime:   runtime.Kind(),
			HomePath:  strings.TrimSpace(homePath),
			Enabled:   true,
			IsDefault: !parsed.ExplicitName || parsed.ResolvedName() == coremodel.DefaultComponentName(parsed.Type),
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
		if strings.TrimSpace(homePath) != "" {
			registration.HomePath = strings.TrimSpace(homePath)
		}
	}

	if err := s.Storage.Components().Save(ctx, registration); err != nil {
		return nil, err
	}
	runtime, err := s.Runtime(registration.Runtime)
	if err != nil {
		return nil, err
	}
	home := runtime.ComponentHome(*registration)
	if err := os.MkdirAll(home.Path, 0o755); err != nil {
		return nil, err
	}
	s.loadedMu.Lock()
	delete(s.loaded, registration.ID.String())
	s.loadedMu.Unlock()
	return registration, nil
}

func (s *System) SetChatWorkspace(ctx context.Context, chatID modeluuid.UUID, workspaceName string) (*coremodel.Chat, error) {
	if s == nil || s.Storage == nil {
		return nil, fmt.Errorf("missing system storage")
	}
	if chatID.IsNull() {
		return nil, fmt.Errorf("missing chat id")
	}
	chat, err := s.Storage.Chats().GetByID(ctx, chatID)
	if err != nil {
		return nil, err
	}
	if chat == nil {
		return nil, fmt.Errorf("chat not found: %s", chatID)
	}
	workspaceName = strings.TrimSpace(workspaceName)
	if workspaceName != "" {
		if _, err := s.Workspace(workspaceName); err != nil {
			return nil, err
		}
	}
	chat.Workspace = workspaceName
	if err := s.Storage.Chats().Save(ctx, chat); err != nil {
		return nil, err
	}
	return chat, nil
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
