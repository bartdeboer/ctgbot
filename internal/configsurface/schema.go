package configsurface

import (
	"fmt"
	"strings"
)

func NormalizeKey(key string) string {
	return strings.ReplaceAll(strings.TrimSpace(strings.ToLower(key)), "_", "-")
}

func (s ConfigSchema) Field(key string) (FieldSchema, bool) {
	key = NormalizeKey(key)
	for _, field := range s.Fields {
		if NormalizeKey(field.Key) == key {
			field.Key = NormalizeKey(field.Key)
			return field, true
		}
	}
	return FieldSchema{}, false
}

func ParseBool(value string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "on", "enabled":
		return true, nil
	case "false", "0", "no", "off", "disabled":
		return false, nil
	default:
		return false, fmt.Errorf("expected true or false")
	}
}
