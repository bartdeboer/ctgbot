package codexengine

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bartdeboer/go-codextgbot/internal/appconfig"
	"github.com/bartdeboer/go-codextgbot/internal/bootstrapassets"
	"github.com/bartdeboer/go-codextgbot/internal/hostbridge"
	"github.com/bartdeboer/go-codextgbot/internal/hostbridgetls"
)

type SessionExecutor struct {
	Config *appconfig.Config
	Logger *log.Logger
}

func (e *SessionExecutor) StartConversation(ctx context.Context, chatID int64, threadID int, workspaceHostPath string) (*ChatSession, error) {
	if e.Config == nil {
		return nil, fmt.Errorf("missing config")
	}
	if err := e.Config.EnsurePaths(); err != nil {
		return nil, err
	}
	if err := e.Config.EnsureCodexCLIHome(); err != nil {
		return nil, err
	}
	builder := &ImageBuilder{Config: e.Config, Logger: e.Logger}
	if err := builder.EnsureImage(ctx); err != nil {
		return nil, err
	}

	workspaceHostPath, err := e.Config.ResolveChatWorkspaceHostPath(chatID, threadID, workspaceHostPath)
	if err != nil {
		return nil, err
	}

	folderName, err := e.Config.EnsureChatRuntimePaths(chatID, threadID)
	if err != nil {
		return nil, err
	}
	containerName := e.Config.ChatContainerName(chatID, threadID)
	homeDir := e.Config.ChatCodexHomeDir(folderName)
	chatTLSDir := e.Config.ChatTLSDir(folderName)
	if err := ensureConversationCodexHome(e.Config, homeDir, e.renderBootstrapInstructions(chatID)); err != nil {
		return nil, err
	}
	if err := hostbridgetls.EnsureChatClientMaterials(e.Config.HostbridgeTLSRoot(), chatTLSDir, containerName); err != nil {
		return nil, fmt.Errorf("ensure hostbridge tls client materials: %w", err)
	}
	if _, err := runCommand(ctx, "", "docker", "rm", "-f", containerName); err != nil {
		e.logf("ignoring stale container cleanup error for %s: %v", containerName, err)
	}

	args := []string{
		"run",
		"-d",
		"--security-opt", "seccomp=unconfined",
		"--name", containerName,
		"--hostname", containerName,
		"--label", "codextgbot.managed=true",
		"--label", fmt.Sprintf("codextgbot.chat_id=%d", chatID),
		"--label", fmt.Sprintf("codextgbot.thread_id=%d", threadID),
		"--env", "HOME=" + e.Config.ContainerHomePath(),
		"--env", "CODEX_HOME=" + e.Config.ContainerHomePath(),
		"--env", "HOSTBRIDGE_ADDR=" + e.Config.ContainerHostbridgeTCPAddr(),
		"--env", "HOSTBRIDGE_TLS_DIR=" + e.Config.ContainerHostbridgeTLSDir(),
		"--workdir", e.Config.ContainerWorkspacePath(),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", workspaceHostPath, e.Config.ContainerWorkspacePath()),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", homeDir, e.Config.ContainerHomePath()),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s,readonly", chatTLSDir, e.Config.ContainerHostbridgeTLSDir()),
	}
	if runtime.GOOS == "linux" {
		args = append(args, "--add-host", "host.docker.internal:host-gateway")
	}

	args = append(args, e.Config.DockerImage(), "tail", "-f", "/dev/null")

	out, err := runCommand(ctx, "", "docker", args...)
	if err != nil {
		return nil, fmt.Errorf("docker run: %w: %s", err, strings.TrimSpace(out))
	}

	e.logf("conversation container started name=%s workspace=%s docker=%s", containerName, workspaceHostPath, strings.TrimSpace(out))

	return &ChatSession{
		ChatID:             chatID,
		ThreadID:           threadID,
		Active:             true,
		ContainerName:      containerName,
		WorkspaceHost:      workspaceHostPath,
		HomeHost:           homeDir,
		ContainerWorkspace: e.Config.ContainerWorkspacePath(),
		ContainerHome:      e.Config.ContainerHomePath(),
	}, nil
}

func (e *SessionExecutor) StopConversation(ctx context.Context, conv *ChatSession) error {
	if conv == nil {
		return nil
	}
	_, err := runCommand(ctx, "", "docker", "rm", "-f", conv.ContainerName)
	return err
}

func (e *SessionExecutor) SendPrompt(ctx context.Context, conv *ChatSession, prompt string) (string, error) {
	if conv == nil {
		return "", fmt.Errorf("missing conversation")
	}
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("missing prompt")
	}

	timeout := e.Config.SessionTimeout()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	outputPath := "/tmp/codextgbot-last-message.txt"
	args := []string{
		"exec",
		"-e", "HOME=" + conv.ContainerHome,
		"-e", "CODEX_HOME=" + conv.ContainerHome,
		"-e", "HOSTBRIDGE_ADDR=" + e.Config.ContainerHostbridgeTCPAddr(),
		"-e", "HOSTBRIDGE_TLS_DIR=" + e.Config.ContainerHostbridgeTLSDir(),
		"-w", conv.ContainerWorkspace,
		conv.ContainerName,
		"codex",
		"-a", "never",
		"-s", "workspace-write",
		"exec",
		"--skip-git-repo-check",
		"--add-dir", conv.ContainerWorkspace,
		"--output-last-message", outputPath,
		"-C", conv.ContainerWorkspace,
	}

	if model := e.Config.CodexModel(); model != "" {
		args = append(args, "-m", model)
	}
	if conv.Initialized {
		args = append(args, "resume", "--last", prompt)
	} else {
		args = append(args, strings.TrimSpace(prompt))
	}

	cmdOut, err := runCommand(ctx, "", "docker", args...)
	lastMessage, readErr := runCommand(ctx, "", "docker", "exec", conv.ContainerName, "cat", outputPath)
	lastMessage = strings.TrimSpace(lastMessage)

	if err != nil {
		if readErr == nil && lastMessage != "" {
			return lastMessage, fmt.Errorf("codex exec: %w", err)
		}
		return "", fmt.Errorf("codex exec: %w: %s", err, strings.TrimSpace(cmdOut))
	}
	if readErr != nil {
		return "", fmt.Errorf("read last message: %w", readErr)
	}
	if lastMessage == "" {
		return "", fmt.Errorf("codex returned an empty response")
	}
	return lastMessage, nil
}

func (e *SessionExecutor) renderBootstrapInstructions(chatID int64) string {
	allowedCommands := strings.Join(hostbridge.AllowedCommandNames(hostbridge.MergeAllowedCommandSpecs(e.Config.ChatHostbridgeAllowedCommandSpecs(chatID))), ", ")
	if strings.TrimSpace(allowedCommands) == "" {
		allowedCommands = "<none>"
	}
	bootstrapText, err := bootstrapassets.Text(bootstrapassets.TemplateData{
		Workspace:      e.Config.ContainerWorkspacePath(),
		CodexHome:      e.Config.ContainerHomePath(),
		HostbridgeAddr: e.Config.ContainerHostbridgeTCPAddr(),
		Binaries:       allowedCommands,
	})
	if err != nil {
		e.logf("render bootstrap template failed: %v", err)
		return ""
	}
	return strings.TrimSpace(bootstrapText)
}

func (e *SessionExecutor) logf(format string, args ...any) {
	if e.Logger != nil {
		e.Logger.Printf(format, args...)
	}
}

func runCommand(ctx context.Context, dir string, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	if dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func cleanTextForTelegram(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.TrimSpace(text)
}

func ensureConversationCodexHome(cfg *appconfig.Config, homeDir string, bootstrapText string) error {
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
	bootstrapPath := filepath.Join(homeDir, "codextgbot-bootstrap.md")
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
`, filepath.Join(cfg.ContainerHomePath(), "codextgbot-bootstrap.md"), cfg.ContainerWorkspacePath())) + "\n"
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
