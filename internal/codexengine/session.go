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
	"runtime"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/bootstrapassets"
	"github.com/bartdeboer/ctgbot/internal/conversationmodel"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridgetls"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

type SessionExecutor struct {
	Config *appconfig.Config
	Logger *log.Logger
}

func (e *SessionExecutor) PrepareConversation(ctx context.Context, conv *conversationmodel.ChatSession) error {
	return e.prepareConversationState(ctx, conv)
}

func (e *SessionExecutor) SandboxSpec(conv *conversationmodel.ChatSession) sandboxengine.Spec {
	spec := sandboxengine.Spec{
		Name:         conv.ContainerName,
		Hostname:     conv.ContainerName,
		Image:        e.Config.DockerImage(),
		Workdir:      conv.ContainerWorkspace,
		SecurityOpts: []string{"seccomp=unconfined"},
		Labels: map[string]string{
			"ctgbot.managed":   "true",
			"ctgbot.chat_id":   fmt.Sprintf("%d", conv.ChatID),
			"ctgbot.thread_id": fmt.Sprintf("%d", conv.ThreadID),
		},
		Env: []string{
			"HOME=" + conv.ContainerHome,
			"CODEX_HOME=" + conv.ContainerHome,
			"HOSTBRIDGE_ADDR=" + e.Config.ContainerHostbridgeTCPAddr(),
			"HOSTBRIDGE_TLS_DIR=" + e.Config.ContainerHostbridgeTLSDir(),
		},
		Mounts: []sandboxengine.Mount{
			{Source: conv.WorkspaceHost, Target: conv.ContainerWorkspace},
			{Source: conv.HomeHost, Target: conv.ContainerHome},
			{Source: e.chatTLSDir(conv), Target: e.Config.ContainerHostbridgeTLSDir(), ReadOnly: true},
		},
		Cmd: []string{"tail", "-f", "/dev/null"},
	}
	if runtime.GOOS == "linux" {
		spec.AddHosts = []string{"host.docker.internal:host-gateway"}
	}
	return spec
}

func (e *SessionExecutor) SendPrompt(ctx context.Context, conv *conversationmodel.ChatSession, prompt string, sbx sandboxengine.Sandbox) (string, error) {
	if conv == nil {
		return "", fmt.Errorf("missing conversation")
	}
	if sbx == nil {
		return "", fmt.Errorf("missing sandbox")
	}
	if strings.TrimSpace(prompt) == "" {
		return "", fmt.Errorf("missing prompt")
	}
	if err := e.prepareConversationState(ctx, conv); err != nil {
		return "", err
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
		"--add-dir", conv.ContainerWorkspace,
		"--output-last-message", outputPath,
		"-C", conv.ContainerWorkspace,
	}

	if model := e.Config.CodexModel(); model != "" {
		args = append(args, "-m", model)
	}
	if conv.Initialized {
		if strings.TrimSpace(conv.ProviderThreadID) != "" {
			args = append(args, "resume", conv.ProviderThreadID, prompt)
		} else {
			args = append(args, "resume", "--last", prompt)
		}
	} else {
		args = append(args, strings.TrimSpace(prompt))
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd := sbx.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)
	err := cmd.Run()

	if conv.ProviderThreadID == "" {
		if threadID := extractCodexThreadID(stdoutBuf.String()); threadID != "" {
			conv.ProviderThreadID = threadID
			e.logf("codex thread started chat=%d thread=%d codex_thread_id=%s", conv.ChatID, conv.ThreadID, threadID)
		}
	}
	lastMessage, readErr := runSandboxCommand(ctx, sbx, "cat", outputPath)
	lastMessage = strings.TrimSpace(lastMessage)

	if err != nil {
		if readErr == nil && lastMessage != "" {
			return lastMessage, fmt.Errorf("codex exec: %w", err)
		}
		detail := strings.TrimSpace(stderrBuf.String())
		if detail == "" {
			detail = strings.TrimSpace(stdoutBuf.String())
		}
		return "", fmt.Errorf("codex exec: %w: %s", err, detail)
	}
	if readErr != nil {
		return "", fmt.Errorf("read last message: %w", readErr)
	}
	if lastMessage == "" {
		return "", fmt.Errorf("codex returned an empty response")
	}
	return lastMessage, nil
}

func (e *SessionExecutor) prepareConversationState(ctx context.Context, conv *conversationmodel.ChatSession) error {
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
	_, err := e.Config.EnsureChatRuntimePaths(conv.ChatID)
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
	conv.HomeHost = e.Config.ChatCodexHomeDirByID(conv.ChatID)
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

func (e *SessionExecutor) chatTLSDir(conv *conversationmodel.ChatSession) string {
	if conv == nil || e.Config == nil || strings.TrimSpace(conv.ContainerName) == "" {
		return ""
	}
	return e.Config.ChatThreadTLSDir(conv.ChatID, conv.ThreadID)
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

func runSandboxCommand(ctx context.Context, sbx sandboxengine.Sandbox, name string, args ...string) (string, error) {
	cmd := sbx.CommandContext(ctx, name, args...)
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
