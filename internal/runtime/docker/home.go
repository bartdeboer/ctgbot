package docker

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

const defaultRuntimeHomePath = "/home/agent"
const bootstrapTimeout = 30 * time.Second

func (r *Runtime) resolveHome(threadID modeluuid.UUID) (string, string, error) {
	if r == nil {
		return "", "", fmt.Errorf("missing docker runtime")
	}
	if threadID.IsNull() {
		return "", "", fmt.Errorf("missing thread id")
	}
	hostPath := threadSandboxHomePath(r.rootDir, threadID)
	if err := prepareHome(hostPath); err != nil {
		return "", "", err
	}
	return hostPath, defaultRuntimeHomePath, nil
}

func threadSandboxHomePath(rootDir string, threadID modeluuid.UUID) string {
	return filepath.Join(rootDir, "threads", threadID.String(), "home")
}

func (r *Runtime) ensureSandboxReady(ctx context.Context, sbx *sandboxengine.Sandbox) error {
	if sbx == nil {
		return fmt.Errorf("missing sandbox")
	}
	action, err := sbx.Ensure(ctx)
	if err != nil {
		return err
	}
	if action == sandboxengine.EnsureNoop {
		return nil
	}
	return r.runHomeBootstrap(ctx, sbx)
}

func (r *Runtime) runHomeBootstrap(ctx context.Context, sbx *sandboxengine.Sandbox) error {
	_ = r
	homePath := strings.TrimSpace(sbx.ContainerHome)
	if homePath == "" {
		return nil
	}
	scriptPath := homePath + "/bootstrap.sh"
	bootstrapCtx, cancel := context.WithTimeout(ctx, bootstrapTimeout)
	defer cancel()
	out, err := sbx.CombinedOutput(bootstrapCtx, "sh", "-lc", "if [ -f '"+scriptPath+"' ]; then sh '"+scriptPath+"'; fi")
	if err == nil {
		return nil
	}
	out = bytes.TrimSpace(out)
	if len(out) == 0 {
		return fmt.Errorf("sandbox home bootstrap: %w", err)
	}
	return fmt.Errorf("sandbox home bootstrap: %w\n%s", err, out)
}

func withHomeEnv(env []string, homePath string) []string {
	homePath = strings.TrimSpace(homePath)
	if homePath == "" {
		homePath = defaultRuntimeHomePath
	}
	path := envValue(env, "PATH")
	if path == "" {
		path = "/usr/local/cuda/bin:/usr/local/nvidia/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
	}
	return runtimepkg.MergeEnv(env, []string{
		"HOME=" + homePath,
		"AGENT_HOME=" + homePath,
		"CTGBOT_AGENT_HOME=" + homePath,
		"PATH=" + homePath + "/bin:" + path,
	})
}

func envValue(env []string, key string) string {
	prefix := key + "="
	for _, value := range env {
		if strings.HasPrefix(value, prefix) {
			return strings.TrimPrefix(value, prefix)
		}
	}
	return ""
}

func prepareHome(hostPath string) error {
	hostPath = strings.TrimSpace(hostPath)
	if hostPath == "" {
		return fmt.Errorf("missing sandbox home host path")
	}
	for _, dir := range []string{"bin", "tools", "services", "state", "cache", "tmp", "logs"} {
		if err := os.MkdirAll(filepath.Join(hostPath, dir), 0o755); err != nil {
			return err
		}
	}
	if err := writeFileIfMissing(filepath.Join(hostPath, "README.md"), []byte(homeReadme)); err != nil {
		return err
	}
	if err := writeFileIfMissing(filepath.Join(hostPath, "bootstrap.sh"), []byte(homeBootstrap)); err != nil {
		return err
	}
	return os.Chmod(filepath.Join(hostPath, "bootstrap.sh"), 0o755)
}

func writeFileIfMissing(path string, content []byte) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

const homeReadme = `# Agent home

This directory is the durable home for this ctgbot sandbox.

- It is mounted at /home/agent inside the container.
- It survives container refresh/recreate.
- Use it for thread-local tools, helper scripts, caches, service configs, logs, and durable runtime state.
- Keep project/repo work in /workspace.
- Keep component identity, auth, and secrets in /profile.

Suggested layout:

- bin/       executable wrappers; /home/agent/bin is added to PATH
- tools/     unpacked userland toolchains or helper binaries
- services/  future supervised service definitions and state
- state/     small durable state files
- cache/     rebuildable caches
- tmp/       temporary files that may still survive refresh
- logs/      local logs

bootstrap.sh runs when ctgbot creates or starts the sandbox. Keep it idempotent, quiet, and fast.
`

const homeBootstrap = `#!/usr/bin/env sh
set -eu

# Lightweight, idempotent sandbox startup preparation.
# Keep output short. Put durable tools under /home/agent/tools and wrappers in /home/agent/bin.
`
