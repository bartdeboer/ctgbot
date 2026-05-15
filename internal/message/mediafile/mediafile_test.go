package mediafile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadDescriptorParsesInlineCIDMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "logo.png")
	if err := os.WriteFile(path, []byte("png"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	media, err := ReadDescriptor(path + ";type=image/png;name=logo.png;cid=logo;disposition=inline")
	if err != nil {
		t.Fatalf("ReadDescriptor() error = %v", err)
	}
	if media.Filename != "logo.png" || media.ContentType != "image/png" || media.ContentID != "logo" || media.Disposition != "inline" || string(media.Content) != "png" {
		t.Fatalf("media = %#v, want inline cid metadata", media)
	}
}

func TestParseDescriptorRejectsInvalidDisposition(t *testing.T) {
	_, err := ParseDescriptor("/tmp/logo.png;disposition=popup")
	if err == nil {
		t.Fatal("ParseDescriptor() error = nil, want invalid disposition error")
	}
}
