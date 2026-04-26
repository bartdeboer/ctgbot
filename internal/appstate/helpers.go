package appstate

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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

func ensureGitWorkspace(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("workspace dir is empty")
	}
	if pathExists(filepath.Join(dir, ".git")) {
		return nil
	}
	cmd := exec.Command("git", "init", "-q", dir)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git init %s: %w: %s", dir, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func readGitConfig(ctx context.Context, key string) string {
	cmd := exec.CommandContext(ctx, "git", "config", "--get", key)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func stringFromAny(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

func firstNonEmptyString(values ...any) string {
	for _, value := range values {
		if s := stringFromAny(value); s != "" {
			return s
		}
	}
	return ""
}

func boolFromAny(v any) bool {
	b, _ := v.(bool)
	return b
}
