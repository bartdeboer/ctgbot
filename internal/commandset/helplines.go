package commandset

import (
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

// HelpOptions controls source, actor, and scope filtering for HelpLines.
type HelpOptions struct {
	Source commandengine.Source
	Actor  coremodel.Actor
	Scope  []string
}

// HelpLines returns help lines for visible, policy-allowed definitions.
// Each line uses "pattern - help" format with no indentation.
// Scope filters to commands whose first visible route starts literally with
// the scope tokens; empty scope returns all commands.
func HelpLines(definitions []commandengine.Definition, opts HelpOptions) []string {
	scope := normalizeHelpScope(opts.Scope)
	var lines []string
	seen := map[string]struct{}{}
	for _, definition := range definitions {
		if opts.Source != "" && !definition.AllowsSource(opts.Source) {
			continue
		}
		if err := definition.Policy.Check(opts.Actor); err != nil {
			continue
		}
		pattern := firstScopedRoutePattern(definition, scope)
		if pattern == "" {
			continue
		}
		line := pattern
		if help := strings.TrimSpace(definition.Help); help != "" {
			line += " - " + help
		}
		if _, ok := seen[line]; ok {
			continue
		}
		seen[line] = struct{}{}
		lines = append(lines, line)
	}
	return lines
}

// firstScopedRoutePattern returns the first visible non-help route pattern that
// starts literally with all scope tokens and has at least one token after them.
// For empty scope it returns the first visible non-help route pattern.
func firstScopedRoutePattern(definition commandengine.Definition, scope []string) string {
	for _, route := range definition.Routes() {
		if route.Hidden {
			continue
		}
		pattern := commandengine.NormalizePattern(route.Pattern)
		if pattern == "" || isHelpLinePattern(pattern) {
			continue
		}
		if len(scope) > 0 && !linePatternMatchesScope(pattern, scope) {
			continue
		}
		return pattern
	}
	return ""
}

// linePatternMatchesScope reports whether pattern has more than len(scope) tokens
// and its first len(scope) tokens match scope exactly (literal, no wildcard).
func linePatternMatchesScope(pattern string, scope []string) bool {
	parts := strings.Fields(pattern)
	if len(parts) <= len(scope) {
		return false
	}
	for i, token := range scope {
		if parts[i] != token {
			return false
		}
	}
	return true
}

func isHelpLinePattern(pattern string) bool {
	return strings.HasSuffix(pattern, " help") || strings.HasSuffix(pattern, " help all")
}

func normalizeHelpScope(scope []string) []string {
	var out []string
	for _, token := range scope {
		if t := strings.TrimSpace(token); t != "" {
			out = append(out, t)
		}
	}
	return out
}
