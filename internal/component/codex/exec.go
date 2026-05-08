package codex

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/containerengine"
	"github.com/bartdeboer/ctgbot/internal/message"
)

const lastMessagePath = "/tmp/ctgbot-last-message.txt"

type ExecRuntime interface {
	Workspace() string
	Exec(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error
	CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error)
}

type OutputHandler interface {
	Send(ctx context.Context, payload message.OutboundPayload) error
}

type TurnRequest struct {
	ProviderThreadID string
	Prompt           string
	Options          TurnOptions
}

type TurnOptions struct {
	Model           string
	ReasoningEffort string
}

type TurnResult struct {
	Reply            string
	ProviderThreadID string
}

type Runner struct {
	Config *appstate.Config
	Logger *log.Logger
}

func NewRunner(cfg *appstate.Config, logger *log.Logger) *Runner {
	return &Runner{Config: cfg, Logger: logger}
}

func (r *Runner) RunTurn(ctx context.Context, runtime ExecRuntime, output OutputHandler, request TurnRequest) (TurnResult, error) {
	if r == nil || r.Config == nil {
		return TurnResult{}, fmt.Errorf("missing config")
	}
	if runtime == nil {
		return TurnResult{}, fmt.Errorf("missing runtime")
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return TurnResult{}, fmt.Errorf("missing prompt")
	}

	if timeout := r.Config.Codex().SessionTimeout(); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	args := BuildExecArgs(ExecArgs{
		Workspace:        runtime.Workspace(),
		OutputPath:       lastMessagePath,
		ProviderThreadID: request.ProviderThreadID,
		Prompt:           prompt,
		DefaultModel:     r.Config.Codex().Model(),
		Options:          request.Options,
	})

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	stdout := newEventWriter(&stdoutBuf, r.logf, func(text string) {
		if output == nil {
			return
		}
		if err := output.Send(ctx, message.OutboundPayload{Text: message.TextMessage{Text: text}}); err != nil {
			r.logf("send codex agent message failed: %v", err)
		}
	})
	err := runtime.Exec(ctx, stdout, io.MultiWriter(os.Stderr, &stderrBuf), args[0], args[1:]...)
	stdout.Flush()

	nextProviderThreadID := strings.TrimSpace(request.ProviderThreadID)
	if nextProviderThreadID == "" {
		nextProviderThreadID = stdout.ThreadID()
	}
	if nextProviderThreadID == "" {
		nextProviderThreadID = extractThreadID(stdoutBuf.String())
	}
	if stdout.InputTokens() > 0 || stdout.OutputTokens() > 0 || stdout.CachedInputTokens() > 0 {
		r.logf("codex turn completed thread_id=%s input_tokens=%d cached_input_tokens=%d output_tokens=%d", nextProviderThreadID, stdout.InputTokens(), stdout.CachedInputTokens(), stdout.OutputTokens())
	}

	if err != nil && errors.Is(err, context.Canceled) && ctx.Err() == nil {
		return TurnResult{ProviderThreadID: nextProviderThreadID}, context.Canceled
	}

	lastMessageBytes, readErr := runtime.CombinedOutput(ctx, "cat", lastMessagePath)
	lastMessage := strings.TrimSpace(string(lastMessageBytes))

	if err != nil {
		if readErr == nil && lastMessage != "" {
			return TurnResult{Reply: lastMessage, ProviderThreadID: nextProviderThreadID}, fmt.Errorf("codex exec: %w", err)
		}
		if detail := trimErrorDetail(stderrBuf.String()); detail != "" {
			return TurnResult{}, fmt.Errorf("codex exec: %w: %s", err, detail)
		}
		return TurnResult{}, fmt.Errorf("codex exec: %w", err)
	}
	if readErr != nil {
		return TurnResult{}, fmt.Errorf("read last message: %w", readErr)
	}
	if lastMessage == "" {
		return TurnResult{}, fmt.Errorf("codex returned an empty response")
	}
	return TurnResult{Reply: lastMessage, ProviderThreadID: nextProviderThreadID}, nil
}

type ExecArgs struct {
	Workspace        string
	OutputPath       string
	ProviderThreadID string
	Prompt           string
	DefaultModel     string
	Options          TurnOptions
}

func BuildExecArgs(request ExecArgs) []string {
	workspace := strings.TrimSpace(request.Workspace)
	outputPath := strings.TrimSpace(request.OutputPath)
	if outputPath == "" {
		outputPath = lastMessagePath
	}
	innerArgs := []string{
		"codex",
		"-a", "never",
		"-s", "workspace-write",
		"exec",
		"--json",
		"--skip-git-repo-check",
		"--add-dir", workspace,
		"--output-last-message", outputPath,
		"-C", workspace,
	}

	model := strings.TrimSpace(request.Options.Model)
	if model == "" {
		model = strings.TrimSpace(request.DefaultModel)
	}
	if model != "" {
		innerArgs = append(innerArgs, "-m", model)
	}
	if effort := strings.TrimSpace(request.Options.ReasoningEffort); effort != "" {
		innerArgs = append(innerArgs, "-c", fmt.Sprintf("model_reasoning_effort=%q", effort))
	}
	providerThreadID := strings.TrimSpace(request.ProviderThreadID)
	prompt := strings.TrimSpace(request.Prompt)
	if providerThreadID != "" {
		innerArgs = append(innerArgs, "resume", providerThreadID, prompt)
	} else {
		innerArgs = append(innerArgs, prompt)
	}
	return wrapWithPIDFile(innerArgs)
}

func wrapWithPIDFile(args []string) []string {
	wrapped := []string{"sh", "-lc", "rm -f " + containerengine.ActivePIDFile + "; echo $$ > " + containerengine.ActivePIDFile + "; exec \"$@\"", "sh"}
	return append(wrapped, args...)
}

func (r *Runner) logf(format string, args ...any) {
	if r != nil && r.Logger != nil {
		r.Logger.Printf(format, args...)
	}
}
