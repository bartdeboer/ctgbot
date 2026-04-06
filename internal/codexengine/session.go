package codexengine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/bootstrapassets"
	"github.com/bartdeboer/ctgbot/internal/chatbroker"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

type SessionExecutor struct {
	Config *appconfig.Config
	Logger *log.Logger
}

func (e *SessionExecutor) Name() string {
	return "codex"
}

func (e *SessionExecutor) InitSession(ctx context.Context, rt chatbroker.RuntimeContext, sbx sandboxengine.Sandbox) error {
	if e.Config == nil {
		return fmt.Errorf("missing config")
	}
	if err := e.Config.EnsurePaths(); err != nil {
		return err
	}
	if err := e.Config.EnsureCodexCLIHome(); err != nil {
		return err
	}
	if err := (&ImageBuilder{Config: e.Config, Logger: e.Logger}).EnsureImage(context.Background()); err != nil {
		return err
	}
	allowedCommands := strings.Join(rt.AllowedHostCommands, ", ")
	if strings.TrimSpace(allowedCommands) == "" {
		allowedCommands = "<none>"
	}
	bootstrapText, err := bootstrapassets.Text(bootstrapassets.TemplateData{
		Workspace:      rt.ContainerWorkspace,
		CodexHome:      rt.ContainerHome,
		ContainerOS:    "linux",
		HostOS:         rt.HostOS,
		HostbridgeAddr: rt.HostbridgeAddr,
		Binaries:       allowedCommands,
	})
	if err != nil {
		e.logf("render bootstrap template failed: %v", err)
		bootstrapText = ""
	}
	return ensureConversationCodexHome(e.Config, rt.ProfilePath, rt.ContainerHome, rt.ContainerWorkspace, bootstrapText)
}

func (e *SessionExecutor) SandboxSpec(rt chatbroker.RuntimeContext) sandboxengine.Spec {
	return sandboxengine.Spec{
		Name:     rt.SandboxName,
		Hostname: rt.SandboxName,
		Image:    e.Config.DockerImage(),
		Workdir:  rt.ContainerWorkspace,
		Env: []string{
			"HOME=" + rt.ContainerHome,
			"CODEX_HOME=" + rt.ContainerHome,
		},
		Mounts: []sandboxengine.Mount{
			{Source: rt.WorkspacePath, Target: rt.ContainerWorkspace},
			{Source: rt.ProfilePath, Target: rt.ContainerHome},
		},
		Cmd: []string{"tail", "-f", "/dev/null"},
	}
}

func (e *SessionExecutor) HandleTurn(ctx context.Context, rt chatbroker.RuntimeContext, sbx sandboxengine.Sandbox, providerThreadID string, prompt string) (chatbroker.TurnResult, error) {
	if sbx == nil {
		return chatbroker.TurnResult{}, fmt.Errorf("missing sandbox")
	}
	if strings.TrimSpace(prompt) == "" {
		return chatbroker.TurnResult{}, fmt.Errorf("missing prompt")
	}

	timeout := e.Config.SessionTimeout()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	outputPath := "/tmp/ctgbot-last-message.txt"
	args := []string{
		"codex",
		"-a", "never",
		"-s", "workspace-write",
		"exec",
		"--json",
		"--skip-git-repo-check",
		"--add-dir", rt.ContainerWorkspace,
		"--output-last-message", outputPath,
		"-C", rt.ContainerWorkspace,
	}

	if model := e.Config.CodexModel(); model != "" {
		args = append(args, "-m", model)
	}
	if strings.TrimSpace(providerThreadID) != "" {
		args = append(args, "resume", providerThreadID, prompt)
	} else {
		args = append(args, strings.TrimSpace(prompt))
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd := sbx.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	err := cmd.Run()

	nextProviderThreadID := strings.TrimSpace(providerThreadID)
	if nextProviderThreadID == "" {
		nextProviderThreadID = extractCodexThreadID(stdoutBuf.String())
	}
	if nextProviderThreadID != "" {
		e.logf("codex thread started provider_thread_id=%s", nextProviderThreadID)
	}
	lastMessage, readErr := runSandboxCommand(ctx, sbx, "cat", outputPath)
	lastMessage = strings.TrimSpace(lastMessage)

	if err != nil {
		if readErr == nil && lastMessage != "" {
			return chatbroker.TurnResult{Reply: lastMessage, ProviderThreadID: nextProviderThreadID}, fmt.Errorf("codex exec: %w", err)
		}
		detail := strings.TrimSpace(stderrBuf.String())
		if detail == "" {
			detail = strings.TrimSpace(stdoutBuf.String())
		}
		return chatbroker.TurnResult{}, fmt.Errorf("codex exec: %w: %s", err, detail)
	}
	if readErr != nil {
		return chatbroker.TurnResult{}, fmt.Errorf("read last message: %w", readErr)
	}
	if lastMessage == "" {
		return chatbroker.TurnResult{}, fmt.Errorf("codex returned an empty response")
	}
	return chatbroker.TurnResult{Reply: lastMessage, ProviderThreadID: nextProviderThreadID}, nil
}

func (e *SessionExecutor) logf(format string, args ...any) {
	if e.Logger != nil {
		e.Logger.Printf(format, args...)
	}
}

func runSandboxCommand(ctx context.Context, sbx sandboxengine.Sandbox, name string, args ...string) (string, error) {
	cmd := sbx.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func ensureConversationCodexHome(cfg *appconfig.Config, homeDir string, containerHome string, containerWorkspace string, bootstrapText string) error {
	if cfg == nil {
		return fmt.Errorf("missing config")
	}
	if err := cfg.EnsureCodexCLIHome(); err != nil {
		return err
	}
	for _, dir := range []string{homeDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	target := filepath.Join(homeDir, "auth.json")
	if !fileExistsAndNonEmpty(target) && fileExistsAndNonEmpty(cfg.CodexCLIHomeAuthPath()) {
		if err := copyFile(cfg.CodexCLIHomeAuthPath(), target); err != nil {
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
writable_roots = [%q]
network_access = true
`, path.Join(containerHome, "ctgbot-bootstrap.md"), containerWorkspace)) + "\n"
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
