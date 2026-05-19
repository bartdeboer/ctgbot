// Package configsurface defines a small shared contract for component-owned
// configuration command surfaces.
//
// The package intentionally does not own storage. Implementations may store
// values in clistate, database columns, JSON blobs, component profile files, or
// domain-specific tables. The shared contract is only the user/agent command
// vocabulary: config list/get/set, with optional unset support.
package configsurface

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

// ConfigSurface is implemented by scopes/components that expose a standard
// config command surface.
//
// ConfigGet returns a display-safe effective value. Secret values should be
// redacted by the implementation unless it deliberately exposes a privileged
// raw-read command elsewhere.
//
// ConfigSet stores an explicit override. Implementations should reject
// ConfigSet for non-writable fields.
type ConfigSurface interface {
	ConfigSchema(ctx context.Context, req commandengine.Request) (ConfigSchema, error)

	ConfigGet(ctx context.Context, req commandengine.Request, key string) (value string, err error)
	ConfigSet(ctx context.Context, req commandengine.Request, key string, value string) error
}

// ConfigUnsetter is implemented by config surfaces that can remove an explicit
// override and fall back to their default/effective value.
type ConfigUnsetter interface {
	ConfigUnset(ctx context.Context, req commandengine.Request, key string) error
}

// ConfigSchema describes the keys exposed by a ConfigSurface.
type ConfigSchema struct {
	Fields []FieldSchema
}

// FieldType is a small display/validation hint for config fields.
type FieldType string

const (
	FieldTypeString     FieldType = "string"
	FieldTypeBool       FieldType = "bool"
	FieldTypeInt        FieldType = "int"
	FieldTypeFloat      FieldType = "float"
	FieldTypeEnum       FieldType = "enum"
	FieldTypeURL        FieldType = "url"
	FieldTypeDuration   FieldType = "duration"
	FieldTypeStringList FieldType = "string-list"
)

// FieldSchema describes one config key.
type FieldSchema struct {
	Key      string
	Help     string
	Type     FieldType
	Writable bool
	Secret   bool
	Default  string
	Options  []string
}
