package server

import (
	"fmt"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/durationparse"
)

type AllowedCommand struct {
	Name           string            `json:"name"`
	Args           []string          `json:"args"`
	ArgsPattern    string            `json:"args_pattern,omitempty"`
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
	planArgs, err := buildPlanArgs(commandName, spec, args)
	if err != nil {
		return ExecutionPlan{}, err
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
	spec.ArgsPattern = strings.TrimSpace(spec.ArgsPattern)
	spec.Dir = strings.TrimSpace(spec.Dir)
	spec.Delay = strings.TrimSpace(spec.Delay)
	spec.Args = cleanCommandArgs(spec.Args)
	spec.Env = cleanCommandEnv(spec.Env)
	if spec.Name == "" {
		return AllowedCommand{}, false
	}
	return spec, true
}

func buildPlanArgs(commandName string, spec AllowedCommand, runtimeArgs []string) ([]string, error) {
	if strings.TrimSpace(spec.ArgsPattern) == "" {
		if hasArgTemplate(spec.Args) {
			return nil, fmt.Errorf("command %s uses argument templates without args_pattern", commandName)
		}
		planArgs := append([]string{}, spec.Args...)
		if len(runtimeArgs) > 0 {
			if !spec.AllowExtraArgs {
				return nil, fmt.Errorf("command does not allow extra args: %s", commandName)
			}
			planArgs = append(planArgs, runtimeArgs...)
		}
		return planArgs, nil
	}
	params, extraArgs, err := matchArgsPattern(commandName, spec.ArgsPattern, runtimeArgs)
	if err != nil {
		return nil, err
	}
	planArgs, err := renderCommandArgs(commandName, spec.Args, params)
	if err != nil {
		return nil, err
	}
	if len(extraArgs) > 0 {
		if !spec.AllowExtraArgs {
			return nil, fmt.Errorf("command does not allow extra args: %s", commandName)
		}
		planArgs = append(planArgs, extraArgs...)
	}
	return planArgs, nil
}

var (
	argsPatternParamRE = regexp.MustCompile(`^<([A-Za-z_][A-Za-z0-9_]*)>$`)
	argTemplateRE      = regexp.MustCompile(`\{\{([A-Za-z_][A-Za-z0-9_]*)\}\}`)
)

func matchArgsPattern(commandName string, pattern string, args []string) (map[string]string, []string, error) {
	tokens := strings.Fields(strings.TrimSpace(pattern))
	if len(args) < len(tokens) {
		return nil, nil, fmt.Errorf("command %s expects %d args, got %d", commandName, len(tokens), len(args))
	}
	params := map[string]string{}
	for i, token := range tokens {
		value := args[i]
		if match := argsPatternParamRE.FindStringSubmatch(token); len(match) == 2 {
			name := match[1]
			if previous, ok := params[name]; ok && previous != value {
				return nil, nil, fmt.Errorf("command %s argument %s was provided more than once with different values", commandName, name)
			}
			params[name] = value
			continue
		}
		if token != value {
			return nil, nil, fmt.Errorf("command %s expects arg %d to be %q", commandName, i+1, token)
		}
	}
	return params, append([]string{}, args[len(tokens):]...), nil
}

func renderCommandArgs(commandName string, args []string, params map[string]string) ([]string, error) {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		rendered, err := renderCommandArg(commandName, arg, params)
		if err != nil {
			return nil, err
		}
		out = append(out, rendered)
	}
	return out, nil
}

func renderCommandArg(commandName string, arg string, params map[string]string) (string, error) {
	if !strings.Contains(arg, "{{") {
		return arg, nil
	}
	missing := ""
	rendered := argTemplateRE.ReplaceAllStringFunc(arg, func(token string) string {
		match := argTemplateRE.FindStringSubmatch(token)
		if len(match) != 2 {
			missing = token
			return token
		}
		value, ok := params[match[1]]
		if !ok {
			missing = match[1]
			return token
		}
		return value
	})
	if missing != "" {
		return "", fmt.Errorf("command %s has unresolved argument template %q", commandName, missing)
	}
	if strings.Contains(rendered, "{{") || strings.Contains(rendered, "}}") {
		return "", fmt.Errorf("command %s has malformed argument template %q", commandName, arg)
	}
	return rendered, nil
}

func hasArgTemplate(args []string) bool {
	for _, arg := range args {
		if strings.Contains(arg, "{{") || strings.Contains(arg, "}}") {
			return true
		}
	}
	return false
}

func cleanCommandArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	out = append(out, args...)
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
