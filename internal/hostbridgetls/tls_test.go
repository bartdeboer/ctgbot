package hostbridgetls

import (
	"path/filepath"
	"testing"
)

func TestEnsureServerAndClientMaterials(t *testing.T) {
	root := t.TempDir()
	serverRoot := filepath.Join(root, "server")
	chatDir := filepath.Join(root, "chat", "tls")

	if err := EnsureServerMaterials(serverRoot); err != nil {
		t.Fatalf("EnsureServerMaterials: %v", err)
	}
	if _, err := LoadServerTLSConfig(serverRoot); err != nil {
		t.Fatalf("LoadServerTLSConfig: %v", err)
	}

	if err := EnsureChatClientMaterials(serverRoot, chatDir, "codextgbot-chat-1"); err != nil {
		t.Fatalf("EnsureChatClientMaterials: %v", err)
	}
	if _, err := LoadClientTLSConfig(chatDir); err != nil {
		t.Fatalf("LoadClientTLSConfig: %v", err)
	}
}
