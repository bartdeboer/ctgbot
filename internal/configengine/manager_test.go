package configengine

import (
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestManagerEnforcesItemPolicyAndScope(t *testing.T) {
	var stored string
	manager := New(MustRegistry(Item{
		Key:         "chat.enabled",
		Scope:       ScopeChat,
		ValueType:   ValueBool,
		ReadPolicy:  simplerbac.Any(simplerbac.RoleUser),
		WritePolicy: simplerbac.Any(simplerbac.RoleRoot),
		Get: func(ctx commandengine.Context) (Value, error) {
			return String(stored), nil
		},
		Set: func(ctx commandengine.Context, value Value) (Value, error) {
			stored = value.String()
			return String(stored), nil
		},
	}))

	userCtx := commandengine.Context{
		ChatID: modeluuid.New(),
		Actor:  commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleUser}},
	}
	if _, err := manager.Set(userCtx, "chat.enabled", "true"); err == nil || !strings.Contains(err.Error(), "set chat.enabled denied") {
		t.Fatalf("Set() user error = %v, want denial", err)
	}

	rootCtx := commandengine.Context{
		ChatID: userCtx.ChatID,
		Actor:  commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}},
	}
	if _, err := manager.Set(rootCtx, "chat.enabled", "true"); err != nil {
		t.Fatalf("Set() root error = %v", err)
	}
	if stored != "true" {
		t.Fatalf("stored = %q, want true", stored)
	}
	if _, err := manager.Get(commandengine.Context{Actor: userCtx.Actor}, "chat.enabled"); err == nil || !strings.Contains(err.Error(), "requires chat id") {
		t.Fatalf("Get() without chat error = %v, want scope error", err)
	}
}
