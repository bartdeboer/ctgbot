package server

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

type RunCommandRunner struct {
	ResolveAllowed    AllowedCommandResolver
	ClientIdentity    string
	DefaultTimeoutSec int
}

func RegisterRunCommandHandler(registry *commandengine.Registry, runner *RunCommandRunner) error {
	return commandengine.Register[schemacommands.RunCommand](registry, runner.RunCommand)
}

func (r *RunCommandRunner) RunCommand(ctx context.Context, req commandengine.Request, cmd schemacommands.RunCommand) (commandengine.Result, error) {
	text, err := r.run(ctx, cmd.Command, cmd.Args, cmd.Stdin, cmd.Timeout)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: text}, nil
}

func (r *RunCommandRunner) run(ctx context.Context, commandName string, args []string, stdin []byte, timeoutSec int) (string, error) {
	allowed := StaticAllowedCommandResolver(nil)("")
	if r != nil && r.ResolveAllowed != nil {
		allowed = r.ResolveAllowed(r.ClientIdentity)
	}
	if allowed == nil {
		allowed = DefaultAllowedCommands()
	}
	spec, ok := allowed[commandName]
	if !ok {
		return "", fmt.Errorf("command not allowed: %s", commandName)
	}

	plan, err := BuildExecutionPlan(commandName, args, spec)
	if err != nil {
		return "", err
	}

	timeout := r.defaultTimeoutSec(timeoutSec)
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	command := exec.CommandContext(runCtx, plan.Name, plan.Args...)
	command.Dir = plan.Dir
	command.Env = plan.Env
	command.Stdin = bytes.NewReader(stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if plan.Delay > 0 {
		select {
		case <-time.After(plan.Delay):
		case <-runCtx.Done():
			return "", runCtx.Err()
		}
	}

	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return "", fmt.Errorf("%w: %s", err, detail)
		}
		return "", err
	}

	text := stdout.String()
	if strings.TrimSpace(text) == "" {
		text = stderr.String()
	}
	return text, nil
}

func (r *RunCommandRunner) defaultTimeoutSec(timeout int) int {
	if timeout > 0 {
		return timeout
	}
	if r != nil && r.DefaultTimeoutSec > 0 {
		return r.DefaultTimeoutSec
	}
	return 30
}
