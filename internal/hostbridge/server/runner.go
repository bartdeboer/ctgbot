package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

const defaultRunCommandTimeoutSec = 60

type RunCommandRunner struct {
	ResolveAliases    AliasResolver
	ClientIdentity    string
	DefaultTimeoutSec int
}

func RegisterRunCommandHandler(registry *commandengine.Registry, runner *RunCommandRunner) error {
	return commandengine.Register[schemacommands.RunCommand](registry, runner.RunCommand)
}

func (r *RunCommandRunner) RunCommand(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error) {
	execution, err := r.run(ctx, cmd.Command, cmd.Args, req.Stdin, cmd.Timeout)
	if execution == nil {
		return commandengine.Result{}, err
	}
	result := commandengine.Result{
		// Keep the pre-outcome projection for in-process callers that still read
		// Text. Hostbridge transports use Execution as the lossless authority.
		Text:      executionText(*execution),
		Execution: execution,
	}
	return result, err
}

func executionText(execution commandengine.ExecutionResult) string {
	text := execution.Stdout
	if strings.TrimSpace(text) == "" {
		text = execution.Stderr
	}
	return text
}

func (r *RunCommandRunner) run(ctx context.Context, commandName string, args []string, stdin string, timeoutSec int) (*commandengine.ExecutionResult, error) {
	aliases := StaticAliasResolver(nil)("")
	if r != nil && r.ResolveAliases != nil {
		aliases = r.ResolveAliases(r.ClientIdentity)
	}
	if aliases == nil {
		aliases = DefaultAliases()
	}
	spec, ok := aliases[commandName]
	if !ok {
		return nil, fmt.Errorf("hostbridge alias not allowed: %s", commandName)
	}

	plan, err := BuildExecutionPlan(commandName, args, spec)
	if err != nil {
		return nil, err
	}
	if err := validateAliasStdin(commandName, spec, stdin); err != nil {
		return nil, err
	}

	timeout := r.defaultTimeoutSec(timeoutSec)
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	command := exec.CommandContext(runCtx, plan.Name, plan.Args...)
	command.Dir = plan.Dir
	command.Env = plan.Env
	command.Stdin = strings.NewReader(stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if plan.Delay > 0 {
		select {
		case <-time.After(plan.Delay):
		case <-runCtx.Done():
			return nil, runCtx.Err()
		}
	}

	err = command.Run()
	execution := commandengine.ExecutionResult{
		Stdout: stdout.String(),
		Stderr: stderr.String(),
	}
	if err == nil {
		return &execution, nil
	}
	if runCtx.Err() != nil {
		return nil, runCtx.Err()
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if exitCode, ok := processExitCode(exitErr); ok {
			execution.ExitCode = exitCode
			return &execution, errors.New(executionFailureMessage(execution))
		}
	}
	return nil, err
}

func processExitCode(err *exec.ExitError) (int, bool) {
	if code := err.ExitCode(); code >= 0 {
		return code, true
	}
	status, ok := err.Sys().(syscall.WaitStatus)
	if !ok || !status.Signaled() {
		return 0, false
	}
	// Match the conventional shell representation while preserving a single,
	// portable integer exit field for callers.
	return 128 + int(status.Signal()), true
}

func executionFailureMessage(execution commandengine.ExecutionResult) string {
	message := fmt.Sprintf("exit status %d", execution.ExitCode)
	detail := strings.TrimSpace(execution.Stderr)
	if detail == "" {
		detail = strings.TrimSpace(execution.Stdout)
	}
	if detail == "" {
		return message
	}
	return message + ": " + detail
}

func validateAliasStdin(commandName string, spec Alias, stdin string) error {
	if stdin == "" {
		return nil
	}
	if spec.StdinMaxBytes <= 0 {
		return fmt.Errorf("hostbridge alias does not allow stdin: %s", commandName)
	}
	if int64(len(stdin)) > spec.StdinMaxBytes {
		return fmt.Errorf("hostbridge alias stdin exceeds %d bytes: %s", spec.StdinMaxBytes, commandName)
	}
	return nil
}

func (r *RunCommandRunner) defaultTimeoutSec(timeout int) int {
	if timeout > 0 {
		return timeout
	}
	if r != nil && r.DefaultTimeoutSec > 0 {
		return r.DefaultTimeoutSec
	}
	return defaultRunCommandTimeoutSec
}
