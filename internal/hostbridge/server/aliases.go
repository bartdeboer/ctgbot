package server

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/durationparse"
	hostbridgepolicy "github.com/bartdeboer/ctgbot/internal/hostbridgepolicy"
)

type Alias = hostbridgepolicy.Alias

type AliasResolver func(clientIdentity string) map[string]Alias

type ExecutionPlan struct {
	Name  string
	Args  []string
	Dir   string
	Delay time.Duration
	Env   []string
}

func BuildExecutionPlan(commandName string, args []string, spec Alias) (ExecutionPlan, error) {
	spec = hostbridgepolicy.NormalizeAlias(spec)
	if spec.Name == "" {
		return ExecutionPlan{}, fmt.Errorf("hostbridge alias %q has empty executable name", commandName)
	}
	if err := hostbridgepolicy.ValidateAlias(spec); err != nil {
		return ExecutionPlan{}, fmt.Errorf("hostbridge alias %q is invalid: %w", commandName, err)
	}
	dir, args, err := resolveWorkingDirectory(commandName, spec, args)
	if err != nil {
		return ExecutionPlan{}, err
	}
	delay, err := parseAliasDelay(commandName, spec.Delay)
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
		Dir:   dir,
		Delay: delay,
		Env:   sanitizedEnv(spec.Env),
	}, nil
}

func DefaultAliases() map[string]Alias {
	aliases := map[string]Alias{}
	if runtime.GOOS == "windows" {
		return aliases
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
			aliases[pair.name] = Alias{Name: pair.path, AllowExtraArgs: true}
		}
	}
	return aliases
}

func MergeExecutableAliases(extra map[string]string) map[string]Alias {
	aliases := DefaultAliases()
	for name, executable := range extra {
		name = strings.TrimSpace(name)
		executable = strings.TrimSpace(executable)
		if name == "" || executable == "" {
			continue
		}
		aliases[name] = Alias{Name: executable}
	}
	return aliases
}

func MergeAliases(extra map[string]Alias) map[string]Alias {
	aliases := DefaultAliases()
	for name, spec := range extra {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if normalized, ok := normalizeAlias(spec); ok {
			aliases[name] = normalized
		}
	}
	return aliases
}

func AliasNames(aliases map[string]Alias) []string {
	if len(aliases) == 0 {
		return nil
	}
	names := make([]string, 0, len(aliases))
	for name := range aliases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func AliasUsages(aliases map[string]Alias) []string {
	if len(aliases) == 0 {
		return nil
	}
	names := AliasNames(aliases)
	out := make([]string, 0, len(names))
	for _, name := range names {
		spec := aliases[name]
		normalized, ok := normalizeAlias(spec)
		if !ok {
			continue
		}
		usage := name
		if len(normalized.AllowedCWDs) > 0 {
			usage += " --cwd <path>"
		}
		if len(normalized.Subcommands) > 0 {
			usage += " [ " + strings.Join(subcommandNames(normalized.Subcommands), " | ") + " ]"
		}
		out = append(out, usage)
	}
	return out
}

func StaticAliasResolver(aliases map[string]Alias) AliasResolver {
	if aliases == nil {
		aliases = DefaultAliases()
	}
	return func(string) map[string]Alias { return aliases }
}

func normalizeAlias(spec Alias) (Alias, bool) {
	spec = hostbridgepolicy.NormalizeAlias(spec)
	if hostbridgepolicy.ValidateAlias(spec) != nil {
		return Alias{}, false
	}
	return spec, true
}

func resolveWorkingDirectory(commandName string, spec Alias, args []string) (string, []string, error) {
	if len(spec.AllowedCWDs) == 0 {
		return spec.Dir, append([]string{}, args...), nil
	}
	if spec.Dir != "" {
		return "", nil, fmt.Errorf("command %s cannot combine dir with allowed_cwds", commandName)
	}
	if len(args) < 2 || args[0] != "--cwd" {
		return "", nil, fmt.Errorf("command %s requires --cwd <path>", commandName)
	}

	requested, err := canonicalWorkingDirectory(args[1])
	if err != nil {
		return "", nil, fmt.Errorf("command %s working directory is not allowed", commandName)
	}
	for _, allowed := range spec.AllowedCWDs {
		candidate, err := canonicalWorkingDirectory(allowed)
		if err != nil {
			continue
		}
		if requested == candidate {
			return requested, append([]string{}, args[2:]...), nil
		}
	}
	return "", nil, fmt.Errorf("command %s working directory is not allowed", commandName)
}

func canonicalWorkingDirectory(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is empty")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be absolute")
	}
	resolved, err := filepath.EvalSymlinks(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("path is not a directory")
	}
	return resolved, nil
}

// workspaceHostPath translates the working directory seen by a sandbox into
// the corresponding host directory. Absolute host paths remain valid for
// compatibility; relative paths are interpreted from the sandbox workspace.
func workspaceHostPath(hostRoot string, runtimeRoot string, requested string) (string, bool, error) {
	hostRoot = strings.TrimSpace(hostRoot)
	runtimeRoot = strings.TrimSpace(runtimeRoot)
	requested = strings.TrimSpace(requested)
	if hostRoot == "" || runtimeRoot == "" || requested == "" {
		return requested, false, nil
	}

	relative := ""
	switch {
	case path.IsAbs(requested):
		if !path.IsAbs(runtimeRoot) {
			return requested, false, nil
		}
		runtimeRoot = path.Clean(runtimeRoot)
		requested = path.Clean(requested)
		switch {
		case requested == runtimeRoot:
			relative = "."
		case strings.HasPrefix(requested, runtimeRoot+"/"):
			relative = strings.TrimPrefix(requested, runtimeRoot+"/")
		default:
			return requested, false, nil
		}
	case filepath.IsAbs(requested):
		return requested, false, nil
	default:
		relative = path.Clean(requested)
		if escapesWorkspace(relative) {
			return "", false, fmt.Errorf("working directory escapes workspace")
		}
	}

	root, err := canonicalWorkingDirectory(hostRoot)
	if err != nil {
		return "", false, err
	}
	candidate, err := canonicalWorkingDirectory(filepath.Join(root, filepath.FromSlash(relative)))
	if err != nil {
		return "", false, err
	}
	relativeToRoot, err := filepath.Rel(root, candidate)
	if err != nil || escapesWorkspace(filepath.ToSlash(relativeToRoot)) {
		return "", false, fmt.Errorf("working directory escapes workspace")
	}
	return candidate, true, nil
}

func escapesWorkspace(relative string) bool {
	return relative == ".." || strings.HasPrefix(relative, "../")
}

func buildPlanArgs(commandName string, spec Alias, runtimeArgs []string) ([]string, error) {
	if len(spec.Subcommands) > 0 {
		return buildSubcommandPlanArgs(commandName, spec, runtimeArgs)
	}
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

func buildSubcommandPlanArgs(commandName string, spec Alias, runtimeArgs []string) ([]string, error) {
	if strings.TrimSpace(spec.ArgsPattern) != "" {
		return nil, fmt.Errorf("command %s cannot combine args_pattern with subcommands", commandName)
	}
	if hasArgTemplate(spec.Args) {
		return nil, fmt.Errorf("command %s uses argument templates without args_pattern", commandName)
	}
	if len(runtimeArgs) == 0 {
		return nil, fmt.Errorf("command %s expects one of: %s", commandName, strings.Join(subcommandNames(spec.Subcommands), ", "))
	}
	subcommandName := strings.TrimSpace(runtimeArgs[0])
	subcommand, ok := spec.Subcommands[subcommandName]
	if !ok {
		return nil, fmt.Errorf("subcommand not allowed for %s: %s", commandName, subcommandName)
	}

	planArgs := append([]string{}, spec.Args...)
	subArgs, extraArgs, err := buildSubcommandArgs(commandName, subcommandName, subcommand, runtimeArgs[1:])
	if err != nil {
		return nil, err
	}
	planArgs = append(planArgs, subArgs...)
	if len(extraArgs) > 0 {
		if !spec.AllowExtraArgs && !subcommand.AllowExtraArgs {
			return nil, fmt.Errorf("command does not allow extra args: %s %s", commandName, subcommandName)
		}
		planArgs = append(planArgs, extraArgs...)
	}
	return planArgs, nil
}

func buildSubcommandArgs(commandName string, subcommandName string, subcommand hostbridgepolicy.AliasSubcommand, runtimeArgs []string) ([]string, []string, error) {
	templateArgs := append([]string{}, subcommand.Args...)
	if len(templateArgs) == 0 {
		templateArgs = []string{subcommandName}
	}
	if strings.TrimSpace(subcommand.ArgsPattern) == "" {
		if hasArgTemplate(templateArgs) {
			return nil, nil, fmt.Errorf("command %s %s uses argument templates without args_pattern", commandName, subcommandName)
		}
		return templateArgs, append([]string{}, runtimeArgs...), nil
	}
	params, extraArgs, err := matchArgsPattern(commandName+" "+subcommandName, subcommand.ArgsPattern, runtimeArgs)
	if err != nil {
		return nil, nil, err
	}
	rendered, err := renderCommandArgs(commandName+" "+subcommandName, templateArgs, params)
	if err != nil {
		return nil, nil, err
	}
	return rendered, extraArgs, nil
}

func subcommandNames(subcommands map[string]hostbridgepolicy.AliasSubcommand) []string {
	names := make([]string, 0, len(subcommands))
	for name := range subcommands {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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

func parseAliasDelay(commandName string, raw string) (time.Duration, error) {
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
