package server

import (
	"fmt"
	"os"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/durationparse"
)

type AllowedCommand struct {
	Name           string            `json:"name"`
	Args           []string          `json:"args"`
	Dir            string            `json:"dir"`
	Delay          string            `json:"delay"`
	Env            map[string]string `json:"env"`
	AllowExtraArgs bool              `json:"allow_extra_args"`
}

type AllowedCommandResolver func(clientIdentity string) map[string]AllowedCommand

type ExecutionPlan struct {
	Name  string
	Args  []string
	Dir   string
	Delay time.Duration
	Env   []string
}

func BuildExecutionPlan(commandName string, args []string, spec AllowedCommand) (ExecutionPlan, error) {
	spec, ok := normalizeAllowedCommand(spec)
	if !ok {
		return ExecutionPlan{}, fmt.Errorf("allowed command %q has empty executable name", commandName)
	}
	delay, err := parseAllowedCommandDelay(commandName, spec.Delay)
	if err != nil {
		return ExecutionPlan{}, err
	}
	planArgs := append([]string{}, spec.Args...)
	if len(args) > 0 {
		if !spec.AllowExtraArgs {
			return ExecutionPlan{}, fmt.Errorf("command does not allow extra args: %s", commandName)
		}
		planArgs = append(planArgs, args...)
	}
	return ExecutionPlan{
		Name:  spec.Name,
		Args:  planArgs,
		Dir:   spec.Dir,
		Delay: delay,
		Env:   sanitizedEnv(spec.Env),
	}, nil
}

func DefaultAllowedCommands() map[string]AllowedCommand {
	allowed := map[string]AllowedCommand{}
	if runtime.GOOS == "windows" {
		return allowed
	}
	for _, pair := range []struct {
		name string
		path string
	}{
		{name: "ls", path: "/bin/ls"},
		{name: "pwd", path: "/bin/pwd"},
		{name: "whoami", path: "/usr/bin/whoami"},
		{name: "uname", path: "/usr/bin/uname"},
	} {
		if _, err := os.Stat(pair.path); err == nil {
			allowed[pair.name] = AllowedCommand{Name: pair.path, AllowExtraArgs: true}
		}
	}
	return allowed
}

func MergeAllowedCommands(extra map[string]string) map[string]AllowedCommand {
	allowed := DefaultAllowedCommands()
	for name, executable := range extra {
		name = strings.TrimSpace(name)
		executable = strings.TrimSpace(executable)
		if name == "" || executable == "" {
			continue
		}
		allowed[name] = AllowedCommand{Name: executable}
	}
	return allowed
}

func MergeNamedAllowedCommands(extra map[string]AllowedCommand) map[string]AllowedCommand {
	allowed := DefaultAllowedCommands()
	for name, spec := range extra {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if normalized, ok := normalizeAllowedCommand(spec); ok {
			allowed[name] = normalized
		}
	}
	return allowed
}

func AllowedCommandNames(allowed map[string]AllowedCommand) []string {
	if len(allowed) == 0 {
		return nil
	}
	names := make([]string, 0, len(allowed))
	for name := range allowed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func StaticAllowedCommandResolver(allowed map[string]AllowedCommand) AllowedCommandResolver {
	if allowed == nil {
		allowed = DefaultAllowedCommands()
	}
	return func(string) map[string]AllowedCommand { return allowed }
}

func normalizeAllowedCommand(spec AllowedCommand) (AllowedCommand, bool) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Dir = strings.TrimSpace(spec.Dir)
	spec.Delay = strings.TrimSpace(spec.Delay)
	spec.Args = cleanCommandArgs(spec.Args)
	spec.Env = cleanCommandEnv(spec.Env)
	if spec.Name == "" {
		return AllowedCommand{}, false
	}
	return spec, true
}

func cleanCommandArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for _, arg := range args {
		out = append(out, arg)
	}
	return out
}

func cleanCommandEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for key, value := range env {
		key = strings.TrimSpace(key)
		if key == "" || strings.ContainsRune(key, '=') {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func sanitizedEnv(extra map[string]string) []string {
	base := append([]string{}, os.Environ()...)
	for k, v := range extra {
		if strings.TrimSpace(k) == "" || strings.ContainsRune(k, '=') {
			continue
		}
		base = upsertEnv(base, k, v)
	}
	return base
}

func upsertEnv(env []string, key string, value string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func parseAllowedCommandDelay(commandName string, raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	d, err := durationparse.Parse(raw, time.Millisecond)
	if err != nil {
		return 0, fmt.Errorf("invalid delay %q for command %s: %w", raw, commandName, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("invalid delay %q for command %s: must be >= 0", raw, commandName)
	}
	return d, nil
}
