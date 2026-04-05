package codexengine

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/bootstrapassets"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridgetls"
)

type SessionExecutor struct {
	Config *appconfig.Config
	Logger *log.Logger
}

type containerState string

const (
	containerMissing containerState = ""
	containerCreated containerState = "created"
	containerRunning containerState = "running"
	containerExited  containerState = "exited"
)

func (e *SessionExecutor) StartConversation(ctx context.Context, chatID int64, threadID int, workspaceHostPath string) (*ChatSession, error) {
	if e.Config == nil {
		return nil, fmt.Errorf("missing config")
	}
	workspaceHostPath, err := e.Config.ResolveChatWorkspaceHostPath(chatID, threadID, workspaceHostPath)
	if err != nil {
		return nil, err
	}

	conv := &ChatSession{
		ChatID:             chatID,
		ThreadID:           threadID,
		Active:             true,
		ContainerName:      e.Config.ChatContainerName(chatID, threadID),
		WorkspaceHost:      workspaceHostPath,
		ContainerWorkspace: e.Config.ContainerWorkspacePath(),
		ContainerHome:      e.Config.ContainerHomePath(),
	}
	if err := e.prepareConversationState(ctx, conv); err != nil {
		return nil, err
	}
	conv.HomeHost = e.Config.ChatCodexHomeDir(e.Config.ChatFolderName(chatID, threadID))
	if err := e.removeContainer(ctx, conv.ContainerName); err != nil {
		e.logf("ignoring stale container cleanup error for %s: %v", conv.ContainerName, err)
	}
	e.logf("conversation session prepared name=%s workspace=%s", conv.ContainerName, conv.WorkspaceHost)
	return conv, nil
}

func (e *SessionExecutor) StopConversation(ctx context.Context, conv *ChatSession) error {
	if conv == nil {
		return nil
	}
	return e.removeContainer(ctx, conv.ContainerName)
}

func (e *SessionExecutor) SendPrompt(ctx context.Context, conv *ChatSession, prompt string) (string, error) {
	if conv == nil {
		return "", fmt.Errorf("missing conversation")
	}
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("missing prompt")
	}
	if err := e.prepareConversationState(ctx, conv); err != nil {
		return "", err
	}
	if err := e.ensureContainerRunning(ctx, conv); err != nil {
		return "", err
	}
	defer func() {
		if err := e.stopContainer(context.Background(), conv.ContainerName); err != nil {
			e.logf("stop conversation container %s failed: %v", conv.ContainerName, err)
		}
	}()

	timeout := e.Config.SessionTimeout()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	outputPath := "/tmp/ctgbot-last-message.txt"
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

func (e *SessionExecutor) prepareConversationState(ctx context.Context, conv *ChatSession) error {
	if e.Config == nil {
		return fmt.Errorf("missing config")
	}
	if conv == nil {
		return fmt.Errorf("missing conversation")
	}
	if err := e.Config.EnsurePaths(); err != nil {
		return err
	}
	if err := e.Config.EnsureCodexCLIHome(); err != nil {
		return err
	}
	builder := &ImageBuilder{Config: e.Config, Logger: e.Logger}
	if err := builder.EnsureImage(ctx); err != nil {
		return err
	}
	folderName, err := e.Config.EnsureChatRuntimePaths(conv.ChatID, conv.ThreadID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(conv.WorkspaceHost) == "" {
		workspaceHostPath, err := e.Config.ResolveChatWorkspaceHostPath(conv.ChatID, conv.ThreadID, "")
		if err != nil {
			return err
		}
		conv.WorkspaceHost = workspaceHostPath
	}
	conv.HomeHost = e.Config.ChatCodexHomeDir(folderName)
	if strings.TrimSpace(conv.ContainerName) == "" {
		conv.ContainerName = e.Config.ChatContainerName(conv.ChatID, conv.ThreadID)
	}
	if strings.TrimSpace(conv.ContainerWorkspace) == "" {
		conv.ContainerWorkspace = e.Config.ContainerWorkspacePath()
	}
	if strings.TrimSpace(conv.ContainerHome) == "" {
		conv.ContainerHome = e.Config.ContainerHomePath()
	}
	if err := ensureConversationCodexHome(e.Config, conv.HomeHost, e.renderBootstrapInstructions(conv.ChatID)); err != nil {
		return err
	}
	if err := hostbridgetls.EnsureChatClientMaterials(e.Config.HostbridgeTLSRoot(), e.chatTLSDir(conv), conv.ContainerName); err != nil {
		return fmt.Errorf("ensure hostbridge tls client materials: %w", err)
	}
	return nil
}

func (e *SessionExecutor) ensureContainerRunning(ctx context.Context, conv *ChatSession) error {
	state, err := e.inspectContainerState(ctx, conv.ContainerName)
	if err != nil {
		return err
	}
	switch state {
	case containerRunning:
		return nil
	case containerCreated, containerExited:
		if _, err := runCommand(ctx, "", "docker", "start", conv.ContainerName); err != nil {
			return fmt.Errorf("docker start %s: %w", conv.ContainerName, err)
		}
		e.logf("conversation container started name=%s", conv.ContainerName)
		return nil
	case containerMissing:
		if err := e.createContainer(ctx, conv); err != nil {
			return err
		}
		if _, err := runCommand(ctx, "", "docker", "start", conv.ContainerName); err != nil {
			return fmt.Errorf("docker start %s: %w", conv.ContainerName, err)
		}
		e.logf("conversation container recreated and started name=%s", conv.ContainerName)
		return nil
	default:
		return fmt.Errorf("unsupported container state %q for %s", state, conv.ContainerName)
	}
}

func (e *SessionExecutor) createContainer(ctx context.Context, conv *ChatSession) error {
	args := []string{
		"create",
		"--security-opt", "seccomp=unconfined",
		"--name", conv.ContainerName,
		"--hostname", conv.ContainerName,
		"--label", "ctgbot.managed=true",
		"--label", fmt.Sprintf("ctgbot.chat_id=%d", conv.ChatID),
		"--label", fmt.Sprintf("ctgbot.thread_id=%d", conv.ThreadID),
		"--env", "HOME=" + conv.ContainerHome,
		"--env", "CODEX_HOME=" + conv.ContainerHome,
		"--env", "HOSTBRIDGE_ADDR=" + e.Config.ContainerHostbridgeTCPAddr(),
		"--env", "HOSTBRIDGE_TLS_DIR=" + e.Config.ContainerHostbridgeTLSDir(),
		"--workdir", conv.ContainerWorkspace,
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", conv.WorkspaceHost, conv.ContainerWorkspace),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", conv.HomeHost, conv.ContainerHome),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s,readonly", e.chatTLSDir(conv), e.Config.ContainerHostbridgeTLSDir()),
	}
	if runtime.GOOS == "linux" {
		args = append(args, "--add-host", "host.docker.internal:host-gateway")
	}
	args = append(args, e.Config.DockerImage(), "tail", "-f", "/dev/null")
	out, err := runCommand(ctx, "", "docker", args...)
	if err != nil {
		return fmt.Errorf("docker create: %w: %s", err, strings.TrimSpace(out))
	}
	e.logf("conversation container created name=%s docker=%s", conv.ContainerName, strings.TrimSpace(out))
	return nil
}

func (e *SessionExecutor) inspectContainerState(ctx context.Context, containerName string) (containerState, error) {
	out, err := runCommand(ctx, "", "docker", "inspect", "-f", "{{.State.Status}}", containerName)
	if err != nil {
		trimmed := strings.TrimSpace(out)
		if strings.Contains(trimmed, "No such object") {
			return containerMissing, nil
		}
		return containerMissing, fmt.Errorf("docker inspect %s: %w: %s", containerName, err, trimmed)
	}
	return containerState(strings.TrimSpace(out)), nil
}

func (e *SessionExecutor) stopContainer(ctx context.Context, containerName string) error {
	state, err := e.inspectContainerState(ctx, containerName)
	if err != nil {
		return err
	}
	if state == containerMissing || state == containerCreated || state == containerExited {
		return nil
	}
	if _, err := runCommand(ctx, "", "docker", "stop", "-t", "1", containerName); err != nil {
		return fmt.Errorf("docker stop %s: %w", containerName, err)
	}
	e.logf("conversation container stopped name=%s", containerName)
	return nil
}

func (e *SessionExecutor) removeContainer(ctx context.Context, containerName string) error {
	state, err := e.inspectContainerState(ctx, containerName)
	if err != nil {
		return err
	}
	if state == containerMissing {
		return nil
	}
	if _, err := runCommand(ctx, "", "docker", "rm", "-f", containerName); err != nil {
		return fmt.Errorf("docker rm -f %s: %w", containerName, err)
	}
	e.logf("conversation container removed name=%s", containerName)
	return nil
}

func (e *SessionExecutor) chatTLSDir(conv *ChatSession) string {
	if conv == nil || strings.TrimSpace(conv.HomeHost) == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(conv.HomeHost), "tls")
}

func (e *SessionExecutor) renderBootstrapInstructions(chatID int64) string {
	allowedCommands := strings.Join(hostbridge.AllowedCommandNames(hostbridge.MergeAllowedCommandSpecs(e.Config.ChatHostbridgeAllowedCommandSpecs(chatID))), ", ")
	if strings.TrimSpace(allowedCommands) == "" {
		allowedCommands = "<none>"
	}
	bootstrapText, err := bootstrapassets.Text(bootstrapassets.TemplateData{
		Workspace:      e.Config.ContainerWorkspacePath(),
		CodexHome:      e.Config.ContainerHomePath(),
		ContainerOS:    "linux",
		HostOS:         runtime.GOOS,
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
`, path.Join(cfg.ContainerHomePath(), "ctgbot-bootstrap.md"), cfg.ContainerWorkspacePath())) + "\n"
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
