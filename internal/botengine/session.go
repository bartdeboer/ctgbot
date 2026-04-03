package botengine

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
	"time"
)

type SessionExecutor struct {
	Config *Config
	Logger *log.Logger
}

func (e *SessionExecutor) StartConversation(ctx context.Context, chatID int64, threadID int, workspaceHostPath string) (*Conversation, error) {
	if e.Config == nil {
		return nil, fmt.Errorf("missing config")
	}
	if err := e.Config.EnsureSharedCodexPaths(); err != nil {
		return nil, err
	}
	builder := &ImageBuilder{Config: e.Config, Logger: e.Logger}
	if err := builder.EnsureImage(ctx); err != nil {
		return nil, err
	}

	workspaceHostPath, err := e.Config.ResolveWorkspaceHostPath(workspaceHostPath)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("codextgbot-%d-%d-%d", chatID, threadID, time.Now().UTC().Unix())
	rootDir := e.Config.ConversationRoot(name)
	homeDir := e.Config.ConversationHomeDir(name)
	logDir := e.Config.ConversationLogDir(name)

	for _, dir := range []string{rootDir, homeDir, logDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}

	args := []string{
		"run",
		"-d",
		"--name", name,
		"--hostname", name,
		"--label", "codextgbot.managed=true",
		"--label", fmt.Sprintf("codextgbot.chat_id=%d", chatID),
		"--label", fmt.Sprintf("codextgbot.thread_id=%d", threadID),
		"--env", "HOME=" + e.Config.ContainerHomePath(),
		"--env", "CODEX_HOME=" + e.Config.ContainerHomePath(),
		"--env", "CODEX_SHARED_HOME=" + e.Config.ContainerSharedCodexPath(),
		"--workdir", e.Config.ContainerWorkspacePath(),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", workspaceHostPath, e.Config.ContainerWorkspacePath()),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", homeDir, e.Config.ContainerHomePath()),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", e.Config.SharedCodexRoot(), e.Config.ContainerSharedCodexPath()),
	}

	hostbridgeSocket := e.Config.HostbridgeSocketPath()
	if hostbridgeSocket != "" {
		args = append(args, "--mount", fmt.Sprintf("type=bind,source=%s,target=%s", hostbridgeSocket, e.Config.ContainerHostbridgeSocketPath()))
	}

	args = append(args, e.Config.DockerImage(), "/usr/local/bin/codextgbot-init", "tail", "-f", "/dev/null")

	out, err := runCommand(ctx, "", "docker", args...)
	if err != nil {
		return nil, fmt.Errorf("docker run: %w: %s", err, strings.TrimSpace(out))
	}

	e.logf("conversation container started name=%s workspace=%s docker=%s", name, workspaceHostPath, strings.TrimSpace(out))

	return &Conversation{
		ChatID:             chatID,
		ThreadID:           threadID,
		Status:             "active",
		ContainerName:      name,
		WorkspaceHost:      workspaceHostPath,
		HomeHost:           homeDir,
		ContainerWorkspace: e.Config.ContainerWorkspacePath(),
		ContainerHome:      e.Config.ContainerHomePath(),
	}, nil
}

func (e *SessionExecutor) StopConversation(ctx context.Context, conv *Conversation) error {
	if conv == nil {
		return nil
	}
	_, err := runCommand(ctx, "", "docker", "rm", "-f", conv.ContainerName)
	return err
}

func (e *SessionExecutor) SendPrompt(ctx context.Context, conv *Conversation, prompt string) (string, error) {
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
		"-w", conv.ContainerWorkspace,
		conv.ContainerName,
		"codex",
		"exec",
		"--skip-git-repo-check",
		"--output-last-message", outputPath,
		"-C", conv.ContainerWorkspace,
	}

	if model := e.Config.CodexModel(); model != "" {
		args = append(args, "-m", model)
	}
	if e.Config.CodexFullAuto() {
		args = append(args, "--full-auto")
	}
	if conv.Initialized {
		args = append(args, "resume", "--last", prompt)
	} else {
		args = append(args, e.bootstrapPrompt(conv, prompt))
	}

	cmdOut, err := runCommand(ctx, "", "docker", args...)
	lastMessage, readErr := runCommand(ctx, "", "docker", "exec", conv.ContainerName, "cat", outputPath)
	lastMessage = strings.TrimSpace(lastMessage)

	if err != nil {
		if lastMessage != "" {
			return lastMessage, fmt.Errorf("codex exec: %w", err)
		}
		if readErr == nil && strings.TrimSpace(lastMessage) != "" {
			return strings.TrimSpace(lastMessage), fmt.Errorf("codex exec: %w", err)
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

func (e *SessionExecutor) bootstrapPrompt(conv *Conversation, userPrompt string) string {
	hostbridge := "The `hostbridge` command is not mounted in this container."
	if e.Config.HostbridgeSocketPath() != "" {
		hostbridge = fmt.Sprintf("If you need host-system access, use the `hostbridge` CLI via the mounted socket at %s.", e.Config.ContainerHostbridgeSocketPath())
	}

	return strings.TrimSpace(fmt.Sprintf(
		"You are replying to a user through a Telegram bot.\nKeep responses concise and practical because long replies will be chunked into Telegram messages.\nYour workspace is mounted at %s.\n%s\n\nUser message:\n%s",
		conv.ContainerWorkspace,
		hostbridge,
		userPrompt,
	))
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
