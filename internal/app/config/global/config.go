// Package global adapts ctgbot's root configengine registry to the shared
// configsurface command contract.
package global

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
)

type Surface struct {
	Manager *configengine.Manager
}

var _ configsurface.ConfigSurface = Surface{}

func New(manager *configengine.Manager) Surface {
	return Surface{Manager: manager}
}

func (s Surface) ConfigSchema(ctx context.Context, req commandengine.Request) (configsurface.ConfigSchema, error) {
	_ = ctx
	if s.Manager == nil {
		return configsurface.ConfigSchema{}, fmt.Errorf("missing config manager")
	}
	items := s.Manager.List(req.Context)
	fields := make([]configsurface.FieldSchema, 0, len(items))
	for _, item := range items {
		fields = append(fields, configsurface.FieldSchema{
			Key:      item.Name(),
			Help:     item.Help,
			Type:     fieldType(item.ValueType),
			Writable: item.Set != nil,
		})
	}
	return configsurface.ConfigSchema{Fields: fields}, nil
}

func (s Surface) ConfigGet(ctx context.Context, req commandengine.Request, key string) (string, error) {
	_ = ctx
	if s.Manager == nil {
		return "", fmt.Errorf("missing config manager")
	}
	value, err := s.Manager.Get(req.Context, key)
	if err != nil {
		return "", err
	}
	return value.String(), nil
}

func (s Surface) ConfigSet(ctx context.Context, req commandengine.Request, key string, value string) error {
	_ = ctx
	if s.Manager == nil {
		return fmt.Errorf("missing config manager")
	}
	_, err := s.Manager.Set(req.Context, key, value)
	return err
}

func fieldType(valueType configengine.ValueType) configsurface.FieldType {
	switch valueType {
	case configengine.ValueBool:
		return configsurface.FieldTypeBool
	case configengine.ValueInt:
		return configsurface.FieldTypeInt
	case configengine.ValueDuration:
		return configsurface.FieldTypeDuration
	case configengine.ValueStringList:
		return configsurface.FieldTypeStringList
	default:
		return configsurface.FieldTypeString
	}
}
