package claude

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type ProfileSpec struct {
	HostProfile   string
	BootstrapText string
}

func PrepareProfile(spec ProfileSpec) error {
	hostProfile := strings.TrimSpace(spec.HostProfile)
	if hostProfile == "" {
		return fmt.Errorf("missing Claude profile path")
	}
	if err := os.MkdirAll(filepath.Join(hostProfile, ".claude"), 0o755); err != nil {
		return err
	}
	return writeBootstrap(hostProfile, spec.BootstrapText)
}

func writeBootstrap(hostProfile string, bootstrapText string) error {
	bootstrapText = strings.TrimSpace(bootstrapText)
	if bootstrapText == "" {
		bootstrapText = "You are Claude Code running inside ctgbot."
	}
	bootstrapPath := filepath.Join(hostProfile, "ctgbot-bootstrap.md")
	return os.WriteFile(bootstrapPath, []byte(bootstrapText+"\n"), 0o600)
}
