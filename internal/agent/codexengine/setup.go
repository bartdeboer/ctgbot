package codexengine

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

func (e *SessionExecutor) SetupEnvironment(ctx context.Context, sbx *sandboxengine.Sandbox) error {
	_ = ctx
	if sbx == nil {
		return fmt.Errorf("missing sandbox")
	}
	if e.Config == nil {
		return fmt.Errorf("missing config")
	}
	if err := e.Config.EnsurePaths(); err != nil {
		return err
	}
	if err := e.Config.Codex().EnsureCLIHome(); err != nil {
		return err
	}
	sbx.ImageBuilder = &ImageBuilder{Config: e.Config, Logger: e.Logger}
	return PrepareConversationHome(e.Config, sbx.ProfileDir, sbx.ContainerHome, sbx.ContainerWorkspace, sbx.DeveloperInstructions)
}

func PrepareConversationHome(cfg *appstate.Config, homeDir string, containerHome string, containerWorkspace string, bootstrapText string) error {
	if cfg == nil {
		return fmt.Errorf("missing config")
	}
	if err := cfg.Codex().EnsureCLIHome(); err != nil {
		return err
	}
	if err := os.MkdirAll(homeDir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(homeDir, "auth.json")
	authPath := cfg.Codex().AuthPath()
	if !fileExistsAndNonEmpty(target) && fileExistsAndNonEmpty(authPath) {
		if err := copyFile(authPath, target); err != nil {
			return err
		}
	}
	bootstrapPath := filepath.Join(homeDir, "ctgbot-bootstrap.md")
	if err := os.WriteFile(bootstrapPath, []byte(strings.TrimSpace(bootstrapText)+"\n"), 0o600); err != nil {
		return err
	}
	configPath := filepath.Join(homeDir, "config.toml")
	configBody := strings.TrimSpace(fmt.Sprintf(`
sandbox_mode = "workspace-write"
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
`, path.Join(containerHome, "ctgbot-bootstrap.md"), containerWorkspace, containerHome, "/tmp")) + "\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		return err
	}
	return nil
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

func copyDirReplace(src string, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("expected directory, got file: %s", src)
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	return copyDir(src, dst)
}

func copyDir(src string, dst string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("expected directory, got file: %s", src)
	}
	if err := os.MkdirAll(dst, info.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())
		info, err := entry.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode().IsRegular():
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		case info.IsDir():
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unsupported skill entry: %s", srcPath)
		}
	}
	return nil
}
