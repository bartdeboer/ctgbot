package copilot

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
)

// CLIVersion is the release whose command definitions and JSONL format we use.
const CLIVersion = "1.0.83"

type ExecRuntime interface {
	Exec(context.Context, io.Writer, io.Writer, string, ...string) error
}

type TurnRequest struct {
	ProviderThreadID  string
	PromptPath        string // Already staged in the runtime's mounted component profile.
	Workspace         string
	Model             string
	SessionTimeoutSec int
}

type Runner struct{}

func (Runner) RunTurn(ctx context.Context, runtime ExecRuntime, request TurnRequest) (TurnResult, error) {
	if runtime == nil {
		return TurnResult{}, fmt.Errorf("missing copilot runtime")
	}
	expected := request.ProviderThreadID
	resume := expected != ""
	if resume {
		var err error
		expected, err = canonicalSessionID(expected)
		if err != nil {
			return TurnResult{}, err
		}
	} else {
		expected = newSessionID()
	}
	args, err := buildExecArgs(request, expected, resume)
	if err != nil {
		return TurnResult{}, err
	}
	timeout := request.SessionTimeoutSec
	if timeout <= 0 {
		timeout = DefaultSessionTimeoutSec
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()
	parser := newEventWriter(expected)
	// Do not log raw provider stderr: it may contain prompts, tokens or employer code.
	stderr := io.Discard
	execErr := runtime.Exec(ctx, parser, stderr, args[0], args[1:]...)
	// Exec synchronously joins its output writers. No detached signaling or readers.
	result, parseErr := parser.finish()
	if ctx.Err() != nil {
		execErr = errors.Join(execErr, ctx.Err())
	}
	if execErr != nil {
		return result, errors.Join(fmt.Errorf("copilot exec: %w", execErr), parseErr)
	}
	return result, parseErr
}

func buildExecArgs(request TurnRequest, expected string, resume bool) ([]string, error) {
	if request.PromptPath == "" || request.Workspace == "" {
		return nil, fmt.Errorf("missing copilot prompt path or workspace")
	}
	for _, value := range []string{request.PromptPath, request.Workspace, request.Model} {
		if strings.ContainsRune(value, 0) {
			return nil, fmt.Errorf("copilot argument contains NUL")
		}
	}
	// The shim redirects a mounted file, then execs the native release binary.
	// Positional arguments preserve quoting and keep prompt contents out of argv.
	args := []string{"sh", "-c", `input=$1; shift; exec "$@" < "$input"`, "sh", request.PromptPath,
		"env", "-u", "GH_TOKEN", "-u", "GITHUB_TOKEN", "copilot", "--output-format=json", "--stream=off", "--no-auto-update", "--no-ask-user",
		"--no-remote-export", "--disable-builtin-mcps", "--allow-tool=read,write,shell",
		"--add-dir=/home/agent", "-C", request.Workspace,
	}
	// Prompt mode disables memory unless --enable-memory is explicitly supplied.
	if request.Model != "" {
		args = append(args, "--model", request.Model)
	}
	if resume {
		args = append(args, "--resume="+expected)
	} else {
		args = append(args, "--session-id="+expected)
	}
	return agentcommon.WrapWithPIDFile(args), nil
}
