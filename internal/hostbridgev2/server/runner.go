package server

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/chatcommands"
	legacyserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	hbprotocol "github.com/bartdeboer/ctgbot/internal/hostbridge/protocol"
)

type AllowedCommandResolver = legacyserver.AllowedCommandResolver

type RunCommandRunner struct {
	ResolveAllowed    AllowedCommandResolver
	ClientIdentity    string
	DefaultTimeoutSec int
}

func NewRunner(resolve AllowedCommandResolver, defaultTimeoutSec int, provider chatcommands.Provider) chatcommands.Runner {
	return NewRunnerForClient(resolve, "", defaultTimeoutSec, provider)
}

func NewRunnerForClient(resolve AllowedCommandResolver, clientIdentity string, defaultTimeoutSec int, provider chatcommands.Provider) chatcommands.Runner {
	return chatcommands.NewDispatchRunner(
		&RunCommandRunner{ResolveAllowed: resolve, ClientIdentity: clientIdentity, DefaultTimeoutSec: defaultTimeoutSec},
		chatcommands.NewProviderRunner(provider),
	)
}

func (r *RunCommandRunner) ExecuteRunCommand(ctx context.Context, req chatcommands.Request, cmd chatcommands.RunCommand) (chatcommands.Result, error) {
	allowed := legacyserver.StaticAllowedCommandResolver(nil)("")
	if r != nil && r.ResolveAllowed != nil {
		allowed = r.ResolveAllowed(r.ClientIdentity)
	}
	if allowed == nil {
		allowed = legacyserver.DefaultAllowedCommands()
	}
	spec, ok := allowed[cmd.Command]
	if !ok {
		return chatcommands.Result{}, fmt.Errorf("command not allowed: %s", cmd.Command)
	}

	plan, err := legacyserver.BuildExecutionPlan(hbprotocol.Request{
		Command: cmd.Command,
		Args:    cmd.Args,
		Timeout: cmd.Timeout,
	}, spec)
	if err != nil {
		return chatcommands.Result{}, err
	}

	timeout := r.defaultTimeoutSec(cmd.Timeout)
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Second)
	defer cancel()

	command := exec.CommandContext(runCtx, plan.Name, plan.Args...)
	command.Dir = plan.Dir
	command.Env = plan.Env
	command.Stdin = bytes.NewReader(cmd.Stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr

	if plan.Delay > 0 {
		select {
		case <-time.After(plan.Delay):
		case <-runCtx.Done():
			return chatcommands.Result{}, runCtx.Err()
		}
	}

	if err := command.Run(); err != nil {
		detail := strings.TrimSpace(stderr.String())
		if detail == "" {
			detail = strings.TrimSpace(stdout.String())
		}
		if detail != "" {
			return chatcommands.Result{}, fmt.Errorf("%w: %s", err, detail)
		}
		return chatcommands.Result{}, err
	}

	text := stdout.String()
	if strings.TrimSpace(text) == "" {
		text = stderr.String()
	}
	return chatcommands.Result{Text: text}, nil
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
