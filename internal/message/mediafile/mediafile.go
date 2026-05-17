package mediafile

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/message"
)

type Descriptor struct {
	Path        string
	Name        string
	ContentType string
	Syntax      string
	ContentID   string
	Disposition string
	Attributes  AttributeDescriptor
}

type AttributeDescriptor struct {
	Width             int
	Height            int
	DurationSeconds   int
	SupportsStreaming bool
	ThumbnailPath     string
}

// ReadDescriptor turns a hostbridge attachment descriptor into message media.
//
// The descriptor format is intentionally small and shared by commands that read
// local files from an agent runtime:
//
//	/path/report.pdf;type=application/pdf;name=report.pdf
//	/path/logo.png;type=image/png;name=logo.png;cid=logo;disposition=inline
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
	thumbnail, err := thumbnailFromDescriptor(descriptor)
	if err != nil {
		return message.Media{}, err
	}
	return message.Media{
		Kind:              "attachment",
		Filename:          filename,
		ContentType:       descriptor.ContentType,
		Syntax:            descriptor.Syntax,
		ContentID:         descriptor.ContentID,
		Disposition:       descriptor.Disposition,
		Content:           append([]byte(nil), content...),
		Width:             descriptor.Attributes.Width,
		Height:            descriptor.Attributes.Height,
		DurationSeconds:   descriptor.Attributes.DurationSeconds,
		SupportsStreaming: descriptor.Attributes.SupportsStreaming,
		Thumbnail:         thumbnail,
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
		case "cid", "content-id":
			descriptor.ContentID = value
		case "disposition":
			switch strings.ToLower(value) {
			case "", "attachment", "inline":
				descriptor.Disposition = strings.ToLower(value)
			default:
				return Descriptor{}, fmt.Errorf("invalid attachment disposition %q", value)
			}
		case "width":
			n, err := parsePositiveInt(key, value)
			if err != nil {
				return Descriptor{}, err
			}
			descriptor.Attributes.Width = n
		case "height":
			n, err := parsePositiveInt(key, value)
			if err != nil {
				return Descriptor{}, err
			}
			descriptor.Attributes.Height = n
		case "duration":
			n, err := parsePositiveInt(key, value)
			if err != nil {
				return Descriptor{}, err
			}
			descriptor.Attributes.DurationSeconds = n
		case "streaming", "supports-streaming":
			b, err := strconv.ParseBool(value)
			if err != nil {
				return Descriptor{}, fmt.Errorf("invalid %s %q", key, value)
			}
			descriptor.Attributes.SupportsStreaming = b
		case "thumbnail":
			descriptor.Attributes.ThumbnailPath = value
		default:
			return Descriptor{}, fmt.Errorf("unknown attachment parameter %q", key)
		}
	}
	return descriptor, nil
}

func parsePositiveInt(name string, value string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid %s %q", name, value)
	}
	return n, nil
}

func thumbnailFromDescriptor(descriptor Descriptor) (*message.MediaThumbnail, error) {
	thumbnailPath := strings.TrimSpace(descriptor.Attributes.ThumbnailPath)
	if thumbnailPath == "" {
		return nil, nil
	}
	content, err := os.ReadFile(thumbnailPath)
	if err != nil {
		return nil, fmt.Errorf("read thumbnail: %w", err)
	}
	return &message.MediaThumbnail{
		Filename:    filepath.Base(thumbnailPath),
		ContentType: "image/jpeg",
		Content:     append([]byte(nil), content...),
	}, nil
}
