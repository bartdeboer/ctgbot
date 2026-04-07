package codexengine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
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

func (e *SessionExecutor) HandleTurn(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string, prompt string) (chatbroker.TurnResult, error) {
	if e.Config == nil {
		return chatbroker.TurnResult{}, fmt.Errorf("missing config")
	}
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

	if err := (&ImageBuilder{Config: e.Config, Logger: e.Logger}).EnsureImage(ctx); err != nil {
		return chatbroker.TurnResult{}, err
	}
	if err := sbx.Ensure(ctx); err != nil {
		return chatbroker.TurnResult{}, err
	}

	outputPath := "/tmp/ctgbot-last-message.txt"
	args := []string{
		"codex",
		"-a", "never",
		"-s", "workspace-write",
		"exec",
		"--json",
		"--skip-git-repo-check",
		"--add-dir", sbx.ContainerWorkspace,
		"--output-last-message", outputPath,
		"-C", sbx.ContainerWorkspace,
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

func runSandboxCommand(ctx context.Context, sbx *sandboxengine.Sandbox, name string, args ...string) (string, error) {
	cmd := sbx.CommandContext(ctx, name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
