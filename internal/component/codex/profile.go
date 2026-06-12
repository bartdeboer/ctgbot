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

type ProfileSpec struct {
	HostProfile      string
	RuntimeProfile   string
	RuntimeWorkspace string
	BootstrapText    string
	SandboxMode      string
}

func PrepareProfile(cfg *appstate.Config, spec ProfileSpec) error {
	if cfg == nil {
		return fmt.Errorf("missing config")
	}
	if err := cfg.Codex().EnsureCLIHome(); err != nil {
		return err
	}
	hostProfile := strings.TrimSpace(spec.HostProfile)
	if hostProfile == "" {
		return fmt.Errorf("missing Codex profile path")
	}
	if err := os.MkdirAll(hostProfile, 0o755); err != nil {
		return err
	}

	if err := importSharedAuth(cfg, hostProfile); err != nil {
		return err
	}
	if err := writeBootstrap(hostProfile, spec.BootstrapText); err != nil {
		return err
	}
	return writeConfig(hostProfile, spec.RuntimeProfile, spec.RuntimeWorkspace, spec.SandboxMode)
}

func importSharedAuth(cfg *appstate.Config, hostProfile string) error {
	target := filepath.Join(hostProfile, "auth.json")
	authPath := cfg.Codex().AuthPath()
	if !fileExistsAndNonEmpty(target) && fileExistsAndNonEmpty(authPath) {
		if err := copyFile(authPath, target); err != nil {
			return err
		}
	}
	return nil
}

func writeBootstrap(hostProfile string, bootstrapText string) error {
	bootstrapText = strings.TrimSpace(bootstrapText)
	if bootstrapText == "" {
		bootstrapText = "You are Codex running inside ctgbot."
	}
	bootstrapPath := filepath.Join(hostProfile, "ctgbot-bootstrap.md")
	return os.WriteFile(bootstrapPath, []byte(bootstrapText+"\n"), 0o600)
}

func writeConfig(hostProfile string, runtimeProfile string, runtimeWorkspace string, sandboxMode string) error {
	runtimeProfile = cleanRuntimePath(runtimeProfile, "/profile")
	runtimeWorkspace = cleanRuntimePath(runtimeWorkspace, "/workspace")
	sandboxMode = strings.TrimSpace(sandboxMode)
	if sandboxMode == "" {
		sandboxMode = DefaultSandboxMode
	}
	configPath := filepath.Join(hostProfile, "config.toml")
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
`, sandboxMode, path.Join(runtimeProfile, "ctgbot-bootstrap.md"), runtimeWorkspace, runtimeProfile, "/tmp")) + "\n"
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
