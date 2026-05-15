package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type HomeSpec struct {
	HostHome      string
	BootstrapText string
}

func PrepareHome(spec HomeSpec) error {
	hostHome := strings.TrimSpace(spec.HostHome)
	if hostHome == "" {
		return fmt.Errorf("missing Claude home path")
	}
	if err := os.MkdirAll(filepath.Join(hostHome, ".claude"), 0o755); err != nil {
		return err
	}
	return writeBootstrap(hostHome, spec.BootstrapText)
}

func writeBootstrap(hostHome string, bootstrapText string) error {
	bootstrapText = strings.TrimSpace(bootstrapText)
	if bootstrapText == "" {
		bootstrapText = "You are Claude Code running inside ctgbot."
	}
	bootstrapPath := filepath.Join(hostHome, "ctgbot-bootstrap.md")
	return os.WriteFile(bootstrapPath, []byte(bootstrapText+"\n"), 0o600)
}
