package configsurface

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

func FormatList(ctx context.Context, req commandengine.Request, surface ConfigSurface, schema ConfigSchema) string {
	if len(schema.Fields) == 0 {
		return "no config keys"
	}
	lines := make([]string, 0, len(schema.Fields))
	for _, field := range schema.Fields {
		value, err := surface.ConfigGet(ctx, req, field.Key)
		if err != nil {
			value = "(error: " + err.Error() + ")"
		}
		lines = append(lines, formatListLine(field, value))
	}
	return strings.Join(lines, "\n")
}

func FormatGet(field FieldSchema, value string) string {
	lines := []string{field.Key + "=" + valueDisplay(field, value)}
	if help := strings.TrimSpace(field.Help); help != "" {
		lines = append(lines, "help: "+help)
	}
	if field.Type != "" {
		lines = append(lines, "type: "+string(field.Type))
	}
	if field.Default != "" {
		lines = append(lines, "default: "+cleanDisplay(defaultDisplay(field)))
	}
	if len(field.Options) > 0 {
		lines = append(lines, "options: "+strings.Join(field.Options, ", "))
	}
	lines = append(lines, "writable: "+boolString(field.Writable))
	return strings.Join(lines, "\n")
}

func formatListLine(field FieldSchema, value string) string {
	line := field.Key + "=" + valueDisplay(field, value)
	var parts []string
	if help := strings.TrimSpace(field.Help); help != "" {
		parts = append(parts, help)
	}
	if def := defaultDisplay(field); def != "" {
		parts = append(parts, "default: "+def)
	}
	if len(field.Options) > 0 {
		parts = append(parts, "options: "+strings.Join(field.Options, ", "))
	}
	if !field.Writable {
		parts = append(parts, "read-only")
	}
	if len(parts) > 0 {
		line += " - " + strings.Join(parts, ". ")
	}
	return line
}

func valueDisplay(field FieldSchema, value string) string {
	if field.Secret && strings.TrimSpace(value) != "" {
		return "[redacted]"
	}
	return cleanDisplay(value)
}

func defaultDisplay(field FieldSchema) string {
	if field.Secret && strings.TrimSpace(field.Default) != "" {
		return "[redacted]"
	}
	return cleanDisplay(field.Default)
}

func cleanDisplay(value string) string {
	return strings.TrimSpace(value)
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
