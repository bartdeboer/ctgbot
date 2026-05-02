package runtime

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v3/coremodel"
)

func (r *Runtime) ResolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error) {
	if r == nil || r.Storage == nil {
		return nil, fmt.Errorf("missing runtime storage")
	}
	parsed, err := coremodel.ParseComponentRef(ref)
	if err != nil {
		return nil, err
	}
	if !parsed.ExplicitName {
		componentRow, err := r.Storage.Components().GetDefaultByType(ctx, parsed.Type)
		if err != nil {
			return nil, err
		}
		if componentRow != nil {
			return componentRow, nil
		}
	}
	componentRow, err := r.Storage.Components().GetByTypeAndName(ctx, parsed.Type, parsed.ResolvedName())
	if err != nil {
		return nil, err
	}
	if componentRow == nil {
		return nil, fmt.Errorf("component not registered: %s", parsed.Ref())
	}
	return componentRow, nil
}

func (r *Runtime) EnsureComponent(ctx context.Context, ref string) (*coremodel.Component, error) {
	if r == nil || r.Storage == nil {
		return nil, fmt.Errorf("missing runtime storage")
	}
	if r.Registry == nil {
		return nil, fmt.Errorf("missing component registry")
	}
	if r.Homes == nil {
		return nil, fmt.Errorf("missing component homes")
	}

	parsed, err := coremodel.ParseComponentRef(ref)
	if err != nil {
		return nil, err
	}
	if _, ok := r.Registry.Factory(parsed.Type); !ok {
		return nil, fmt.Errorf("component type not registered in code: %s", parsed.Type)
	}

	componentRow, err := r.Storage.Components().GetByTypeAndName(ctx, parsed.Type, parsed.ResolvedName())
	if err != nil {
		return nil, err
	}
	if componentRow == nil {
		componentRow = &coremodel.Component{
			Type:      parsed.Type,
			Name:      parsed.ResolvedName(),
			Enabled:   true,
			IsDefault: !parsed.ExplicitName || parsed.ResolvedName() == coremodel.DefaultComponentName(parsed.Type),
		}
	} else if !componentRow.Enabled {
		componentRow.Enabled = true
	}
	if strings.TrimSpace(componentRow.Name) == "" {
		componentRow.Name = parsed.ResolvedName()
	}
	if err := r.Storage.Components().Save(ctx, componentRow); err != nil {
		return nil, err
	}
	if _, err := r.Homes.Ensure(*componentRow); err != nil {
		return nil, err
	}
	return componentRow, nil
}

func (r *Runtime) BindChatComponent(ctx context.Context, chatID modeluuid.UUID, role coremodel.ChatComponentRole, ref string, externalChatID string) (*coremodel.ChatComponent, error) {
	if r == nil || r.Storage == nil {
		return nil, fmt.Errorf("missing runtime storage")
	}
	if chatID.IsNull() {
		return nil, fmt.Errorf("missing chat id")
	}
	if !role.Valid() {
		return nil, fmt.Errorf("invalid chat component role: %q", role)
	}
	chat, err := r.Storage.Chats().GetByID(ctx, chatID)
	if err != nil {
		return nil, err
	}
	if chat == nil {
		return nil, fmt.Errorf("chat not found: %s", chatID)
	}
	componentRow, err := r.ResolveComponentRef(ctx, ref)
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

	binding, err := r.Storage.ChatComponents().GetByChatComponentRole(ctx, chatID, componentRow.ID, role)
	if err != nil {
		return nil, err
	}
	if binding == nil {
		binding = &coremodel.ChatComponent{
			ChatID:      chatID,
			ComponentID: componentRow.ID,
			Role:        role,
		}
	}
	binding.ExternalChatID = externalChatID
	binding.Enabled = true
	if err := r.Storage.ChatComponents().Save(ctx, binding); err != nil {
		return nil, err
	}
	return binding, nil
}
