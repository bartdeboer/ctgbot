package configsurface

import "strings"

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
