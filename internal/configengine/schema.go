package configengine

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type Scope string

const (
	ScopeRoot   Scope = "root"
	ScopeChat   Scope = "chat"
	ScopeThread Scope = "thread"
)

type ValueType string

const (
	ValueString     ValueType = "string"
	ValueBool       ValueType = "bool"
	ValueInt        ValueType = "int"
	ValueDuration   ValueType = "duration"
	ValueStringList ValueType = "string-list"
)

type Value struct {
	Raw string
}

func String(value string) Value {
	return Value{Raw: strings.TrimSpace(value)}
}

func List(values []string) Value {
	return Value{Raw: strings.Join(normalizeList(values), "\n")}
}

func (v Value) String() string {
	return strings.TrimSpace(v.Raw)
}

func (v Value) StringList() []string {
	return normalizeList(strings.FieldsFunc(v.Raw, func(r rune) bool {
		return r == '\n' || r == ','
	}))
}

func (v Value) Bool() (bool, error) {
	switch strings.ToLower(v.String()) {
	case "true", "1", "yes", "on", "enabled":
		return true, nil
	case "false", "0", "no", "off", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("invalid bool %q", v.String())
	}
}

type Getter func(ctx commandengine.Context) (Value, error)
type Setter func(ctx commandengine.Context, value Value) (Value, error)

type Item struct {
	Key       string
	Help      string
	Scope     Scope
	ValueType ValueType

	ReadPolicy  simplerbac.Rule
	WritePolicy simplerbac.Rule

	Get Getter
	Set Setter
}

func (i Item) Name() string {
	return NormalizeKey(i.Key)
}

func (i Item) Validate() error {
	if i.Name() == "" {
		return fmt.Errorf("missing config item key")
	}
	if i.Scope == "" {
		return fmt.Errorf("config item %s has no scope", i.Name())
	}
	if i.ValueType == "" {
		return fmt.Errorf("config item %s has no value type", i.Name())
	}
	if i.Get == nil && i.Set == nil {
		return fmt.Errorf("config item %s has no getter or setter", i.Name())
	}
	return nil
}

func NormalizeKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.ReplaceAll(key, "_", "-")
	return strings.ToLower(key)
}

type Registry struct {
	items map[string]Item
	order []string
}

func NewRegistry(items ...Item) (*Registry, error) {
	r := &Registry{items: map[string]Item{}}
	for _, item := range items {
		if err := r.Register(item); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func MustRegistry(items ...Item) *Registry {
	r, err := NewRegistry(items...)
	if err != nil {
		panic(err)
	}
	return r
}

func (r *Registry) Register(item Item) error {
	if r == nil {
		return fmt.Errorf("missing config registry")
	}
	if err := item.Validate(); err != nil {
		return err
	}
	key := item.Name()
	if _, exists := r.items[key]; exists {
		return fmt.Errorf("duplicate config item: %s", key)
	}
	r.items[key] = item
	r.order = append(r.order, key)
	return nil
}

func (r *Registry) Find(key string) (Item, bool) {
	if r == nil {
		return Item{}, false
	}
	item, ok := r.items[NormalizeKey(key)]
	return item, ok
}

func (r *Registry) List() []Item {
	if r == nil {
		return nil
	}
	out := make([]Item, 0, len(r.order))
	for _, key := range r.order {
		out = append(out, r.items[key])
	}
	return out
}

func (r *Registry) Sorted() []Item {
	out := r.List()
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}

func normalizeList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	return out
}
