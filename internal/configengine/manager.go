package configengine

import (
	"fmt"
	"strconv"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type Manager struct {
	Registry *Registry
}

func New(registry *Registry) *Manager {
	return &Manager{Registry: registry}
}

func (m *Manager) List(ctx commandengine.Context) []Item {
	items := m.allItems()
	if len(items) == 0 {
		return nil
	}
	actor := simplerbac.Actor{Roles: ctx.Actor.Roles}
	out := make([]Item, 0, len(items))
	for _, item := range items {
		if err := requireScope(ctx, item); err != nil {
			continue
		}
		if item.ReadPolicy.Allows(actor) {
			out = append(out, item)
		}
	}
	return out
}

func (m *Manager) Get(ctx commandengine.Context, key string) (Value, error) {
	item, err := m.findReadable(key)
	if err != nil {
		return Value{}, err
	}
	actor := simplerbac.Actor{Roles: ctx.Actor.Roles}
	if err := item.ReadPolicy.Check(actor); err != nil {
		return Value{}, fmt.Errorf("get %s denied: %w", item.Name(), err)
	}
	if err := requireScope(ctx, item); err != nil {
		return Value{}, err
	}
	return item.Get(ctx)
}

func (m *Manager) Set(ctx commandengine.Context, key string, value any) (Value, error) {
	item, err := m.findWritable(key)
	if err != nil {
		return Value{}, err
	}
	actor := simplerbac.Actor{Roles: ctx.Actor.Roles}
	if err := item.WritePolicy.Check(actor); err != nil {
		return Value{}, fmt.Errorf("set %s denied: %w", item.Name(), err)
	}
	if err := requireScope(ctx, item); err != nil {
		return Value{}, err
	}
	return item.Set(ctx, valueFromAny(value))
}

func (m *Manager) allItems() []Item {
	if m == nil || m.Registry == nil {
		return nil
	}
	return m.Registry.Sorted()
}

func (m *Manager) findReadable(key string) (Item, error) {
	if m == nil || m.Registry == nil {
		return Item{}, fmt.Errorf("missing config registry")
	}
	item, ok := m.Registry.Find(key)
	if !ok {
		return Item{}, fmt.Errorf("unknown config item: %s", NormalizeKey(key))
	}
	if item.Get == nil {
		return Item{}, fmt.Errorf("config item is write only: %s", item.Name())
	}
	return item, nil
}

func (m *Manager) findWritable(key string) (Item, error) {
	if m == nil || m.Registry == nil {
		return Item{}, fmt.Errorf("missing config registry")
	}
	item, ok := m.Registry.Find(key)
	if !ok {
		return Item{}, fmt.Errorf("unknown config item: %s", NormalizeKey(key))
	}
	if item.Set == nil {
		return Item{}, fmt.Errorf("config item is read only: %s", item.Name())
	}
	return item, nil
}

func requireScope(ctx commandengine.Context, item Item) error {
	switch item.Scope {
	case ScopeRoot:
		return nil
	case ScopeChat:
		if ctx.ChatID.IsNull() {
			return fmt.Errorf("config item %s requires chat id", item.Name())
		}
		return nil
	case ScopeThread:
		if ctx.ChatID.IsNull() {
			return fmt.Errorf("config item %s requires chat id", item.Name())
		}
		if ctx.ThreadID.IsNull() {
			return fmt.Errorf("config item %s requires thread id", item.Name())
		}
		return nil
	default:
		return fmt.Errorf("config item %s has unsupported scope: %s", item.Name(), item.Scope)
	}
}

func valueFromAny(value any) Value {
	switch v := value.(type) {
	case Value:
		return v
	case string:
		return String(v)
	case []byte:
		return String(string(v))
	case bool:
		return String(strconv.FormatBool(v))
	case fmt.Stringer:
		return String(v.String())
	default:
		return String(fmt.Sprint(v))
	}
}
