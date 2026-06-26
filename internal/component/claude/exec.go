package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
)

type ExecRuntime interface {
	Exec(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error
}

type TurnRequest struct {
	ProviderThreadID string
	Prompt           string
	Options          TurnOptions
}

type TurnOptions struct {
	Model             string
	PermissionMode    string
	SystemPrompt      string
	SessionTimeoutSec int
}

type TurnResult struct {
	Reply            string
	ProviderThreadID string
}

type Runner struct {
	Logger *log.Logger
}

func NewRunner(logger *log.Logger) *Runner { return &Runner{Logger: logger} }

func (r *Runner) RunTurn(ctx context.Context, runtime ExecRuntime, request TurnRequest) (TurnResult, error) {
	if runtime == nil {
		return TurnResult{}, fmt.Errorf("missing runtime")
	}
	prompt := strings.TrimSpace(request.Prompt)
	if prompt == "" {
		return TurnResult{}, fmt.Errorf("missing prompt")
	}
	if timeout := request.Options.timeout(); timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	args := BuildExecArgs(ExecArgs{
		ProviderThreadID: request.ProviderThreadID,
		Prompt:           prompt,
		Options:          request.Options,
	})

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	err := runtime.Exec(ctx, &stdoutBuf, io.MultiWriter(os.Stderr, &stderrBuf), args[0], args[1:]...)
	if err != nil && errors.Is(err, context.Canceled) && ctx.Err() == nil {
		return TurnResult{ProviderThreadID: strings.TrimSpace(request.ProviderThreadID)}, context.Canceled
	}

	parsed, parseErr := parseClaudeOutput(stdoutBuf.String())
	if err != nil {
		if parsed.Reply != "" {
			return parsed, fmt.Errorf("claude exec: %w: %s", err, trimErrorDetail(parsed.Reply))
		}
		if detail := trimErrorDetail(stderrBuf.String()); detail != "" {
			return TurnResult{}, fmt.Errorf("claude exec: %w: %s", err, detail)
		}
		return TurnResult{}, fmt.Errorf("claude exec: %w", err)
	}
	if parseErr != nil {
		return TurnResult{}, parseErr
	}
	if strings.TrimSpace(parsed.Reply) == "" {
		return TurnResult{}, fmt.Errorf("claude returned an empty response")
	}
	return parsed, nil
}

type ExecArgs struct {
	ProviderThreadID string
	Prompt           string
	Options          TurnOptions
}

func BuildExecArgs(request ExecArgs) []string {
	innerArgs := []string{
		"claude",
		"-p", strings.TrimSpace(request.Prompt),
		"--output-format", "json",
		"--exclude-dynamic-system-prompt-sections",
	}
	if model := strings.TrimSpace(request.Options.Model); model != "" {
		innerArgs = append(innerArgs, "--model", model)
	}
	if mode := strings.TrimSpace(request.Options.PermissionMode); mode != "" {
		innerArgs = append(innerArgs, "--permission-mode", mode)
	}
	if systemPrompt := strings.TrimSpace(request.Options.SystemPrompt); systemPrompt != "" {
		innerArgs = append(innerArgs, "--append-system-prompt", systemPrompt)
	}
	if sessionID := strings.TrimSpace(request.ProviderThreadID); sessionID != "" {
		innerArgs = append(innerArgs, "--resume", sessionID)
	}
	return wrapWithPIDFile(innerArgs)
}

func wrapWithPIDFile(args []string) []string {
	wrapped := []string{"sh", "-lc", "rm -f " + containerengine.ActivePIDFile + "; echo $$ > " + containerengine.ActivePIDFile + "; exec \"$@\"", "sh"}
	return append(wrapped, args...)
}

func (o TurnOptions) timeout() time.Duration {
	if o.SessionTimeoutSec <= 0 {
		return time.Duration(DefaultSessionTimeoutSec) * time.Second
	}
	return time.Duration(o.SessionTimeoutSec) * time.Second
}

type claudeOutput struct {
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
	Message   struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"message"`
}

func parseClaudeOutput(text string) (TurnResult, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return TurnResult{}, fmt.Errorf("claude returned no JSON output")
	}
	var out claudeOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		return TurnResult{}, fmt.Errorf("parse claude JSON output: %w", err)
	}
	reply := strings.TrimSpace(out.Result)
	if reply == "" {
		var parts []string
		for _, item := range out.Message.Content {
			if part := strings.TrimSpace(item.Text); part != "" {
				parts = append(parts, part)
			}
		}
		reply = strings.Join(parts, "\n\n")
	}
	return TurnResult{Reply: reply, ProviderThreadID: strings.TrimSpace(out.SessionID)}, nil
}

const errorDetailMax = 4000

func trimErrorDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if len(detail) <= errorDetailMax {
		return detail
	}
	return strings.TrimSpace(detail[:errorDetailMax]) + "..."
}
