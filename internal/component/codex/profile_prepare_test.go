package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/go-clistate"
)

func TestPrepareProfileWritesManagedFilesAndImportsAuth(t *testing.T) {
	withTempCwd(t, func(root string) {
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
		if err != nil {
			t.Fatalf("new config: %v", err)
		}

		sharedAuth := filepath.Join(root, ".ctgbot", ".codex", "auth.json")
		if err := os.MkdirAll(filepath.Dir(sharedAuth), 0o755); err != nil {
			t.Fatalf("mkdir shared auth dir: %v", err)
		}
		if err := os.WriteFile(sharedAuth, []byte(`{"token":"x"}`), 0o600); err != nil {
			t.Fatalf("write shared auth: %v", err)
		}

		profileDir := filepath.Join(root, "components", "codex")
		if err := PrepareProfile(cfg, ProfileSpec{
			HostProfile:      profileDir,
			RuntimeProfile:   "/profile/components/codex/codex",
			RuntimeWorkspace: "/workspace",
			BootstrapText:    "bootstrap text",
		}); err != nil {
			t.Fatalf("PrepareProfile: %v", err)
		}

		for _, name := range []string{"auth.json", "config.toml", "ctgbot-bootstrap.md"} {
			if _, err := os.Stat(filepath.Join(profileDir, name)); err != nil {
				t.Fatalf("%s missing: %v", name, err)
			}
		}
		bootstrap, err := os.ReadFile(filepath.Join(profileDir, "ctgbot-bootstrap.md"))
		if err != nil {
			t.Fatalf("read bootstrap: %v", err)
		}
		if got := strings.TrimSpace(string(bootstrap)); got != "bootstrap text" {
			t.Fatalf("bootstrap text = %q", got)
		}
	})
}

func TestPrepareProfileWritesPosixModelInstructionsPath(t *testing.T) {
	withTempCwd(t, func(root string) {
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
		if err != nil {
			t.Fatalf("new config: %v", err)
		}

		profileDir := filepath.Join(root, "components", "codex")
		if err := PrepareProfile(cfg, ProfileSpec{
			HostProfile:      profileDir,
			RuntimeProfile:   `\\profile\\components\\codex\\codex`,
			RuntimeWorkspace: `\\workspace`,
			BootstrapText:    "bootstrap",
		}); err != nil {
			t.Fatalf("PrepareProfile: %v", err)
		}

		body, err := os.ReadFile(filepath.Join(profileDir, "config.toml"))
		if err != nil {
			t.Fatalf("read config.toml: %v", err)
		}
		text := string(body)
		if !strings.Contains(text, `model_instructions_file = "/profile/components/codex/codex/ctgbot-bootstrap.md"`) {
			t.Fatalf("config.toml missing model instructions path:\n%s", text)
		}
		if !strings.Contains(text, `sandbox_mode = "danger-full-access"`) {
			t.Fatalf("config.toml missing default sandbox mode:\n%s", text)
		}
		if strings.Contains(text, `\\profile`) {
			t.Fatalf("config.toml still contains windows-style paths:\n%s", text)
		}
		if !strings.Contains(text, `writable_roots = ["/workspace", "/profile/components/codex/codex", "/tmp"]`) {
			t.Fatalf("config.toml missing writable roots:\n%s", text)
		}
	})
}

func TestPrepareProfileWritesConfiguredSandboxMode(t *testing.T) {
	withTempCwd(t, func(root string) {
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
		if err != nil {
			t.Fatalf("new config: %v", err)
		}

		profileDir := filepath.Join(root, "components", "codex")
		if err := PrepareProfile(cfg, ProfileSpec{
			HostProfile:      profileDir,
			RuntimeProfile:   "/profile/components/codex/restricted",
			RuntimeWorkspace: "/workspace",
			SandboxMode:      "workspace-write",
		}); err != nil {
			t.Fatalf("PrepareProfile: %v", err)
		}

		body, err := os.ReadFile(filepath.Join(profileDir, "config.toml"))
		if err != nil {
			t.Fatalf("read config.toml: %v", err)
		}
		if !strings.Contains(string(body), `sandbox_mode = "workspace-write"`) {
			t.Fatalf("config.toml missing configured sandbox mode:\n%s", string(body))
		}
	})
}

func TestPrepareProfileWritesNonEmptyDefaultBootstrap(t *testing.T) {
	withTempCwd(t, func(root string) {
		store, err := clistate.NewCwd("ctgbot", "config")
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
		if err != nil {
			t.Fatalf("new config: %v", err)
		}

		profileDir := filepath.Join(root, "components", "codex")
		if err := PrepareProfile(cfg, ProfileSpec{
			HostProfile:      profileDir,
			RuntimeProfile:   "/profile/components/codex/codex",
			RuntimeWorkspace: "/workspace",
		}); err != nil {
			t.Fatalf("PrepareProfile: %v", err)
		}

		body, err := os.ReadFile(filepath.Join(profileDir, "ctgbot-bootstrap.md"))
		if err != nil {
			t.Fatalf("read bootstrap: %v", err)
		}
		if strings.TrimSpace(string(body)) == "" {
			t.Fatalf("bootstrap is empty")
		}
	})
}
