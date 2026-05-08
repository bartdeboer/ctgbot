package codex

import (
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
)

type HomeSpec struct {
	HostHome         string
	RuntimeHome      string
	RuntimeWorkspace string
	BootstrapText    string
}

func PrepareHome(cfg *appstate.Config, spec HomeSpec) error {
	if cfg == nil {
		return fmt.Errorf("missing config")
	}
	if err := cfg.Codex().EnsureCLIHome(); err != nil {
		return err
	}
	hostHome := strings.TrimSpace(spec.HostHome)
	if hostHome == "" {
		return fmt.Errorf("missing Codex home path")
	}
	if err := os.MkdirAll(hostHome, 0o755); err != nil {
		return err
	}

	if err := importSharedAuth(cfg, hostHome); err != nil {
		return err
	}
	if err := writeBootstrap(hostHome, spec.BootstrapText); err != nil {
		return err
	}
	return writeConfig(hostHome, spec.RuntimeHome, spec.RuntimeWorkspace)
}

func importSharedAuth(cfg *appstate.Config, hostHome string) error {
	target := filepath.Join(hostHome, "auth.json")
	authPath := cfg.Codex().AuthPath()
	if !fileExistsAndNonEmpty(target) && fileExistsAndNonEmpty(authPath) {
		if err := copyFile(authPath, target); err != nil {
			return err
		}
	}
	return nil
}

func writeBootstrap(hostHome string, bootstrapText string) error {
	bootstrapPath := filepath.Join(hostHome, "ctgbot-bootstrap.md")
	return os.WriteFile(bootstrapPath, []byte(strings.TrimSpace(bootstrapText)+"\n"), 0o600)
}

func writeConfig(hostHome string, runtimeHome string, runtimeWorkspace string) error {
	runtimeHome = cleanRuntimePath(runtimeHome, "/profile")
	runtimeWorkspace = cleanRuntimePath(runtimeWorkspace, "/workspace")
	configPath := filepath.Join(hostHome, "config.toml")
	configBody := strings.TrimSpace(fmt.Sprintf(`
sandbox_mode = %q
approval_policy = "never"
project_root_markers = []
model_instructions_file = %q

[tools]
web_search = false

[sandbox_workspace_write]
exclude_tmpdir_env_var = false
exclude_slash_tmp = false
writable_roots = [%q, %q, %q]
network_access = true
`, DefaultSandboxMode, path.Join(runtimeHome, "ctgbot-bootstrap.md"), runtimeWorkspace, runtimeHome, "/tmp")) + "\n"
	return os.WriteFile(configPath, []byte(configBody), 0o600)
}

func cleanRuntimePath(value string, fallback string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	if value == "" {
		value = fallback
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return path.Clean(value)
}

func fileExistsAndNonEmpty(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	return info.Size() > 0
}

func copyFile(src, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("expected file, got directory: %s", src)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return os.Chmod(dst, info.Mode().Perm())
}
