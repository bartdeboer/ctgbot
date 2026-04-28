package codexengine

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/agent"
	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/containerengine"
	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

type SessionExecutor struct {
	Config *appstate.Config
	Logger *log.Logger
}

func NewSessionExecutor(cfg *appstate.Config, logger *log.Logger) *SessionExecutor {
	return &SessionExecutor{Config: cfg, Logger: logger}
}

func (e *SessionExecutor) Name() string {
	return "codex"
}

func (e *SessionExecutor) Purge(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string) error {
	_ = ctx
	_ = sbx
	_ = providerThreadID

	// TODO: delete the Codex conversation when Codex exposes supported session
	// deletion through the runtime or CLI.
	return nil
}

func (e *SessionExecutor) InstallSkill(ctx context.Context, sbx *sandboxengine.Sandbox, skillDir string) error {
	_ = ctx
	if sbx == nil {
		return fmt.Errorf("missing sandbox")
	}
	if strings.TrimSpace(sbx.ProfileDir) == "" {
		return fmt.Errorf("missing sandbox profile dir")
	}
	skillDir = strings.TrimSpace(skillDir)
	if skillDir == "" {
		return fmt.Errorf("skill dir is empty")
	}
	if !filepath.IsAbs(skillDir) {
		return fmt.Errorf("skill dir must be absolute: %s", skillDir)
	}
	name := strings.TrimSpace(filepath.Base(skillDir))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return fmt.Errorf("invalid skill dir: %s", skillDir)
	}
	targetDir := filepath.Join(sbx.ProfileDir, "skills", name)
	if err := copyDirReplace(skillDir, targetDir); err != nil {
		return fmt.Errorf("install skill %s: %w", skillDir, err)
	}
	return nil
}

func (e *SessionExecutor) HandleTurn(ctx context.Context, sbx *sandboxengine.Sandbox, output agent.OutputHandler, providerThreadID string, prompt string, options agent.TurnOptions) (agent.TurnResult, error) {
	if e.Config == nil {
		return agent.TurnResult{}, fmt.Errorf("missing config")
	}
	if sbx == nil {
		return agent.TurnResult{}, fmt.Errorf("missing sandbox")
	}
	if strings.TrimSpace(prompt) == "" {
		return agent.TurnResult{}, fmt.Errorf("missing prompt")
	}

	timeout := e.Config.Codex().SessionTimeout()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	outputPath := "/tmp/ctgbot-last-message.txt"
	innerArgs := []string{
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

	model := strings.TrimSpace(options.Model)
	if model == "" {
		model = e.Config.Codex().Model()
	}
	if model != "" {
		innerArgs = append(innerArgs, "-m", model)
	}
	if effort := strings.TrimSpace(options.ReasoningEffort); effort != "" {
		innerArgs = append(innerArgs, "-c", fmt.Sprintf("model_reasoning_effort=%q", effort))
	}
	if strings.TrimSpace(providerThreadID) != "" {
		innerArgs = append(innerArgs, "resume", providerThreadID, prompt)
	} else {
		innerArgs = append(innerArgs, strings.TrimSpace(prompt))
	}
	args := wrapWithPIDFile(innerArgs)

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	stdout := newCodexJSONWriter(&stdoutBuf, e.logf, func(text string) {
		if output == nil {
			return
		}
		if err := output.Send(ctx, messenger.OutboundPayload{
			Text: messenger.TextMessage{Text: text},
		}); err != nil {
			e.logf("send codex agent message failed: %v", err)
		}
	})
	err := sbx.Exec(ctx, stdout, io.MultiWriter(os.Stderr, &stderrBuf), args[0], args[1:]...)
	stdout.Flush()

	nextProviderThreadID := strings.TrimSpace(providerThreadID)
	if nextProviderThreadID == "" {
		nextProviderThreadID = stdout.ThreadID()
	}
	if nextProviderThreadID == "" {
		nextProviderThreadID = extractCodexThreadID(stdoutBuf.String())
	}
	if stdout.InputTokens() > 0 || stdout.OutputTokens() > 0 || stdout.CachedInputTokens() > 0 {
		e.logf("codex turn completed thread_id=%s input_tokens=%d cached_input_tokens=%d output_tokens=%d", nextProviderThreadID, stdout.InputTokens(), stdout.CachedInputTokens(), stdout.OutputTokens())
	}
	if err != nil && sbx.Interrupted() {
		return agent.TurnResult{ProviderThreadID: nextProviderThreadID}, context.Canceled
	}
	lastMessageBytes, readErr := sbx.CombinedOutput(ctx, "cat", outputPath)
	lastMessage := strings.TrimSpace(string(lastMessageBytes))

	if err != nil {
		if readErr == nil && lastMessage != "" {
			return agent.TurnResult{Reply: lastMessage, ProviderThreadID: nextProviderThreadID}, fmt.Errorf("codex exec: %w", err)
		}
		if detail := trimCodexErrorDetail(stderrBuf.String()); detail != "" {
			return agent.TurnResult{}, fmt.Errorf("codex exec: %w: %s", err, detail)
		}
		return agent.TurnResult{}, fmt.Errorf("codex exec: %w", err)
	}
	if readErr != nil {
		return agent.TurnResult{}, fmt.Errorf("read last message: %w", readErr)
	}
	if lastMessage == "" {
		return agent.TurnResult{}, fmt.Errorf("codex returned an empty response")
	}
	return agent.TurnResult{Reply: lastMessage, ProviderThreadID: nextProviderThreadID}, nil
}

func wrapWithPIDFile(args []string) []string {
	wrapped := []string{"sh", "-lc", "rm -f " + containerengine.ActivePIDFile + "; echo $$ > " + containerengine.ActivePIDFile + "; exec \"$@\"", "sh"}
	return append(wrapped, args...)
}

func (e *SessionExecutor) logf(format string, args ...any) {
	if e.Logger != nil {
		e.Logger.Printf(format, args...)
	}
}
