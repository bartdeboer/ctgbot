package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	clientpkg "github.com/bartdeboer/ctgbot/internal/hostbridge/client"
	"github.com/bartdeboer/ctgbot/internal/hostbridge/cmdsurface"
	_ "github.com/bartdeboer/ctgbot/internal/hostbridge/gobregister"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func main() {
	args := normalizedArgs(os.Args[1:], currentComponentRef())
	if len(args) == 0 || (len(args) == 1 && args[0] == "help") {
		printHelp()
		return
	}
	if len(args) == 1 && args[0] == "version" {
		fmt.Fprintln(os.Stdout, buildassets.Version())
		return
	}

	base, err := baseRequest()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	router, err := hostbridgeRouter()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	req, handled, err := parseOrRenderHelp(context.Background(), router, base, args, os.Stdout)
	if handled {
		return
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		printHelp()
		os.Exit(1)
	}

	resp, err := clientpkg.DoCommand(context.Background(), getenv("HOSTBRIDGE_ADDR", "host.docker.internal:4568"), getenv("HOSTBRIDGE_TLS_DIR", ""), hostbridge.CommandRequest{Request: req})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if strings.TrimSpace(resp.Result.Text) != "" {
		fmt.Fprintln(os.Stdout, resp.Result.Text)
	}
}

func parseOrRenderHelp(
	ctx context.Context,
	router *commandengine.Router,
	base commandengine.Request,
	args []string,
	helpWriter io.Writer,
) (commandengine.Request, bool, error) {
	if helpReq, ok := commandengine.ParseHelpRequest(args); ok {
		match, err := router.Match(ctx, args)
		if err != nil {
			return commandengine.Request{}, false, err
		}
		if !match.Matched || !match.Executable || !match.Exact {
			if err := fprintContextualHelp(ctx, router, helpWriter, args, helpReq.Scope); err != nil {
				return commandengine.Request{}, false, err
			}
			return commandengine.Request{}, true, nil
		}
	}

	req, err := router.Parse(ctx, base, args)
	return req, false, err
}

func fprintContextualHelp(ctx context.Context, router *commandengine.Router, w io.Writer, args []string, scope []string) error {
	var buf bytes.Buffer
	if err := router.FPrintHelp(ctx, &buf, args); err != nil {
		return err
	}
	base := buf.String()
	if _, err := io.WriteString(w, base); err != nil {
		return err
	}
	for _, line := range importantHelpLines(router.Definitions(), scope) {
		if strings.Contains(base, line) {
			continue
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}

func importantHelpLines(definitions []commandengine.Definition, scope []string) []string {
	scope = normalizedScope(scope)
	if len(scope) != 1 {
		return nil
	}
	var lines []string
	seen := map[string]struct{}{}
	for _, definition := range definitions {
		visibility := definition.InstructionVisibilityOrDefault()
		if visibility != commandengine.InstructionImportant && visibility != commandengine.InstructionEssential {
			continue
		}
		for _, route := range definition.Routes() {
			if route.Hidden {
				continue
			}
			pattern := commandengine.NormalizePattern(route.Pattern)
			if pattern == "" || isHelpRoute(pattern) || !routeMatchesScope(pattern, scope) {
				continue
			}
			line := pattern
			if strings.TrimSpace(definition.Help) != "" {
				line += " - " + strings.TrimSpace(definition.Help)
			}
			if _, ok := seen[line]; ok {
				continue
			}
			seen[line] = struct{}{}
			lines = append(lines, line)
		}
	}
	return lines
}

func normalizedScope(scope []string) []string {
	var out []string
	for _, token := range scope {
		token = strings.TrimSpace(token)
		if token != "" {
			out = append(out, token)
		}
	}
	return out
}

func routeMatchesScope(pattern string, scope []string) bool {
	parts := strings.Fields(pattern)
	if len(parts) <= len(scope) {
		return false
	}
	for i, token := range scope {
		if !routeTokenMatchesScope(parts[i], token) {
			return false
		}
	}
	return true
}

func routeTokenMatchesScope(routeToken string, scopeToken string) bool {
	if routeToken == scopeToken {
		return true
	}
	return strings.HasPrefix(routeToken, "<") && strings.HasSuffix(routeToken, ">")
}

func isHelpRoute(pattern string) bool {
	return strings.HasSuffix(pattern, " help") || strings.HasSuffix(pattern, " help all")
}

func normalizedArgs(args []string, componentRef string) []string {
	if len(args) == 0 {
		return nil
	}
	if len(args) >= 2 && args[0] == "run" && args[1] == "sendstdin" {
		return append([]string{"sendstdin"}, args[2:]...)
	}
	if isActiveComponentPrefix(args[0]) {
		return args
	}
	if isDirectHostbridgeCommand(args[0], componentRef) {
		return args
	}
	if cmdsurface.LegacyCodexShorthandEnabled(componentRef) && isLegacyCodexShorthand(args[0]) {
		return append([]string{"codex"}, args...)
	}
	return append([]string{"run"}, args...)
}

func isActiveComponentPrefix(arg string) bool {
	for _, prefix := range activeComponentPrefixes() {
		if arg == prefix {
			return true
		}
	}
	return false
}

func activeComponentPrefixes() []string {
	active := strings.TrimSpace(os.Getenv("CTGBOT_ACTIVE_COMPONENTS"))
	if active == "" {
		return nil
	}
	refs := strings.Split(active, ",")
	counts := map[string]int{}
	for _, ref := range refs {
		if typ := componentType(ref); typ != "" {
			counts[typ]++
		}
	}
	var out []string
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		out = append(out, ref)
		if typ := componentType(ref); typ != "" && counts[typ] == 1 {
			out = append(out, typ)
		}
	}
	return out
}

func componentType(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return ""
	}
	if before, _, ok := strings.Cut(ref, "/"); ok {
		return strings.TrimSpace(before)
	}
	return ref
}

func isDirectHostbridgeCommand(arg string, componentRef string) bool {
	switch arg {
	case "", "run", "message", "sendfile", "sendstdin", "config", "help", "version":
		return true
	}
	for _, prefix := range cmdsurface.GlobalDirectPrefixes() {
		if arg == prefix {
			return true
		}
	}
	for _, prefix := range cmdsurface.DirectPrefixes(componentRef) {
		if arg == prefix {
			return true
		}
	}
	return false
}

func isLegacyCodexShorthand(arg string) bool {
	switch arg {
	case "refresh", "purge", "interrupt", "container", "chat", "model":
		return true
	default:
		return false
	}
}

func baseRequest() (commandengine.Request, error) {
	req := commandengine.Request{Context: commandengine.Context{
		Actor: commandengine.Actor{ID: "hostbridge", Roles: []simplerbac.Role{simplerbac.RoleAgent}},
	}}
	sandboxIDText := strings.TrimSpace(os.Getenv("CTGBOT_SANDBOX_ID"))
	if sandboxIDText == "" {
		return req, nil
	}
	sandboxID, err := modeluuid.Parse(sandboxIDText)
	if err != nil {
		return commandengine.Request{}, fmt.Errorf("parse CTGBOT_SANDBOX_ID: %w", err)
	}
	req.Context.SandboxID = sandboxID
	return req, nil
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func printHelp() {
	fmt.Fprintln(os.Stdout, "usage: hostbridge <command> [args...]")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Commands for ctgbot hostbridge:")
	fmt.Fprintln(os.Stdout, "version - Show embedded hostbridge version")
	printDefinitionHelp(hostbridgeDefinitions())
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "environment:")
	fmt.Fprintln(os.Stdout, "  HOSTBRIDGE_ADDR     TCP address (default host.docker.internal:4568)")
	fmt.Fprintln(os.Stdout, "  HOSTBRIDGE_TLS_DIR  Optional directory containing ca.crt, client.crt, client.key")
	fmt.Fprintln(os.Stdout, "  CTGBOT_SANDBOX_ID   Sandbox/thread id for outbound/config commands")
	fmt.Fprintln(os.Stdout, "  CTGBOT_COMPONENT_REF  Current component ref for bound command routing (default codex)")
	fmt.Fprintln(os.Stdout, "  CTGBOT_ACTIVE_COMPONENTS  Comma-separated active command component refs")
	resolved := cmdsurface.Resolve(currentComponentRef())
	if !resolved.Supported && strings.TrimSpace(os.Getenv("CTGBOT_ACTIVE_COMPONENTS")) == "" {
		fmt.Fprintln(os.Stdout, "")
		fmt.Fprintf(os.Stdout, "note: no component-specific hostbridge commands are registered for %s\n", resolved.ComponentRef)
	}
}

func hostbridgeRouter(_ ...[]string) (*commandengine.Router, error) {
	return commandset.NewBoundRouterForSource(
		commandengine.SourceHostbridge,
		hostbridgeBoundSurfaces(),
		append(cmdsurface.GlobalSurfaces(), cmdsurface.ParseOnlySurfaces()...)...,
	)
}

func hostbridgeDefinitions(_ ...[]string) []commandengine.Definition {
	return commandset.DefinitionsForBoundSource(
		commandengine.SourceHostbridge,
		hostbridgeBoundSurfaces(),
		cmdsurface.GlobalSurfaces()...,
	)
}

func hostbridgeBoundSurfaces() []commandset.BoundSurface {
	active := strings.TrimSpace(os.Getenv("CTGBOT_ACTIVE_COMPONENTS"))
	if active == "" {
		return cmdsurface.BoundSurfaces(currentComponentRef())
	}
	var out []commandset.BoundSurface
	for _, ref := range strings.Split(active, ",") {
		out = append(out, cmdsurface.CommandRefBoundSurfaces(ref)...)
	}
	return out
}

func currentComponentRef() string {
	if ref := strings.TrimSpace(os.Getenv("CTGBOT_COMPONENT_REF")); ref != "" {
		return ref
	}
	return cmdsurface.DefaultComponentType
}

func printDefinitionHelp(definitions []commandengine.Definition) {
	for _, definition := range definitions {
		for _, route := range definition.Routes() {
			if route.Hidden {
				continue
			}
			if definition.Help == "" {
				fmt.Fprintf(os.Stdout, "%s\n", route.Pattern)
				continue
			}
			fmt.Fprintf(os.Stdout, "%s - %s\n", route.Pattern, definition.Help)
		}
	}
}
