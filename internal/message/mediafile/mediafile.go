package mediafile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/message"
)

type Descriptor struct {
	Path        string
	Name        string
	ContentType string
	Syntax      string
}

// ReadDescriptor turns a hostbridge attachment descriptor into message media.
//
// The descriptor format is intentionally small and shared by commands that read
// local files from an agent runtime:
//
//	/path/report.pdf;type=application/pdf;name=report.pdf
//
// If the complete descriptor exists as a file path, it wins before parsing
// semicolon parameters. That keeps unusual filenames usable while preserving a
// curl-like syntax for the normal case.
func ReadDescriptor(raw string) (message.Media, error) {
	descriptor, err := ParseDescriptor(raw)
	if err != nil {
		return message.Media{}, err
	}
	content, err := os.ReadFile(descriptor.Path)
	if err != nil {
		return message.Media{}, err
	}
	filename := strings.TrimSpace(descriptor.Name)
	if filename == "" {
		filename = filepath.Base(descriptor.Path)
	}
	return message.Media{
		Kind:        "attachment",
		Filename:    filename,
		ContentType: descriptor.ContentType,
		Syntax:      descriptor.Syntax,
		Content:     append([]byte(nil), content...),
	}, nil
}

func ParseDescriptor(raw string) (Descriptor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return Descriptor{}, fmt.Errorf("missing attachment path")
	}
	if _, err := os.Stat(raw); err == nil {
		return Descriptor{Path: raw}, nil
	}

	parts := strings.Split(raw, ";")
	descriptor := Descriptor{Path: strings.TrimSpace(parts[0])}
	if descriptor.Path == "" {
		return Descriptor{}, fmt.Errorf("missing attachment path")
	}
	for _, part := range parts[1:] {
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			return Descriptor{}, fmt.Errorf("invalid attachment parameter %q", part)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "type":
			descriptor.ContentType = value
		case "syntax":
			descriptor.Syntax = value
		case "name":
			descriptor.Name = value
		default:
			return Descriptor{}, fmt.Errorf("unknown attachment parameter %q", key)
		}
	}
	return descriptor, nil
}
