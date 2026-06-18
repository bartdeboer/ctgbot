package main

import (
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
	hostbridgetls "github.com/bartdeboer/ctgbot/internal/hostbridge/tls"
	gobtransport "github.com/bartdeboer/ctgbot/internal/hostbridge/transport/gob"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func main() {
	args := normalizedArgs(os.Args[1:], currentComponentRef())
	var err error
	args, err = expandStdinArgs(args, os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if len(args) == 0 || (len(args) == 1 && args[0] == "help") {
		printHelp(defaultHostbridgeActor())
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
		printHelp(base.Context.Actor)
		os.Exit(1)
	}

	tlsConfig, err := hostbridgetls.LoadClientTLSConfigIfPresent(getenv("HOSTBRIDGE_TLS_DIR", ""))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	client := clientpkg.New(gobtransport.NewCommandRunner(
		getenv("HOSTBRIDGE_ADDR", "host.docker.internal:4568"),
		tlsConfig,
	))
	resp, err := client.DoCommand(context.Background(), hostbridge.CommandRequest{Request: req})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if strings.TrimSpace(resp.Result.Text) != "" {
		fmt.Fprintln(os.Stdout, resp.Result.Text)
	}
}

func expandStdinArgs(args []string, stdin io.Reader) ([]string, error) {
	switch {
	case len(args) == 1 && args[0] == "send":
		return appendStdinText(args, stdin)
	case len(args) >= 1 && args[0] == "sendfile" && sendfileUsesStdin(args[1:]):
		out := append([]string{"sendstdin"}, args[1:]...)
		return out, nil
	case len(args) == 4 && args[0] == "thread" && args[2] == "message" && args[3] == "send":
		return appendStdinText(args, stdin)
	default:
		return args, nil
	}
}

func sendfileUsesStdin(args []string) bool {
	return len(args) == 0 || strings.HasPrefix(args[0], "-")
}

func appendStdinText(args []string, stdin io.Reader) ([]string, error) {
	data, err := io.ReadAll(stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	text := string(data)
	if strings.TrimSpace(text) == "" {
		return args, nil
	}
	out := append([]string{}, args...)
	out = append(out, text)
	return out, nil
}

func parseOrRenderHelp(
	ctx context.Context,
	router *commandengine.Router,
	base commandengine.Request,
	args []string,
	helpWriter io.Writer,
) (commandengine.Request, bool, error) {
	if helpReq, ok := commandengine.ParseHelpRequest(args); ok {
		if hostbridgeTextCommandHelpShouldWin(helpReq.Scope) {
			renderHostbridgeTextCommandHelp(helpWriter, helpReq.Scope[0])
			return commandengine.Request{}, true, nil
		}
		match, err := router.Match(ctx, args)
		if err != nil {
			return commandengine.Request{}, false, err
		}
		if !match.Matched || !match.Executable || !match.Exact {
			helpArgs := []string(nil)
			helpOptions := []commandengine.HelpOption{commandengine.HelpLitDepth(1)}
			if len(helpReq.Scope) > 0 {
				helpArgs = append([]string{}, helpReq.Scope...)
				helpOptions = []commandengine.HelpOption{commandengine.HelpLitDepth(2)}
			}
			var err error
			if len(helpReq.Scope) == 0 {
				err = router.FPrintHelpIndex(ctx, helpWriter, base.Context.Actor)
			} else {
				err = router.FPrintHelpWithOptions(ctx, helpWriter, helpArgs, helpOptions, base.Context.Actor)
			}
			if err != nil {
				return commandengine.Request{}, false, err
			}
			return commandengine.Request{}, true, nil
		}
	}

	req, err := router.Parse(ctx, base, args)
	return req, false, err
}

func hostbridgeTextCommandHelpShouldWin(scope []string) bool {
	if len(scope) != 1 {
		return false
	}
	switch scope[0] {
	case "send", "sendfile":
		return true
	default:
		return false
	}
}

func renderHostbridgeTextCommandHelp(w io.Writer, command string) {
	switch command {
	case "send":
		fmt.Fprintln(w, "Usage:")
		fmt.Fprintln(w, "  hostbridge send [--type <mime>] [--syntax <language>] <text>")
		fmt.Fprintln(w, "  hostbridge send <text> [--type <mime>] [--syntax <language>]")
		fmt.Fprintln(w, "  hostbridge send stdin [--type <mime>] [--syntax <language>]")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Examples:")
		fmt.Fprintln(w, "  hostbridge send --type text/plain '<b>not bold</b>'")
		fmt.Fprintln(w, "  printf 'hello' | hostbridge send stdin --type text/plain")
	case "sendfile":
		fmt.Fprintln(w, "Usage:")
		fmt.Fprintln(w, "  hostbridge sendfile <path> [--caption <text>] [--type <mime>] [--syntax <language>]")
		fmt.Fprintln(w, "  hostbridge sendfile stdin [--name <filename>] [--caption <text>] [--type <mime>] [--syntax <language>]")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "Examples:")
		fmt.Fprintln(w, "  hostbridge sendfile report.pdf --caption 'Report'")
		fmt.Fprintln(w, "  printf 'hello' | hostbridge sendfile stdin --name note.txt --type text/plain")
	}
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
	case "", "run", "send", "message", "sendfile", "sendstdin", "config", "help", "version":
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
		Actor: defaultHostbridgeActor(),
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

func defaultHostbridgeActor() commandengine.Actor {
	return commandengine.Actor{ID: "hostbridge", Roles: []simplerbac.Role{simplerbac.RoleAgent}}
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func printHelp(actor commandengine.Actor) {
	fmt.Fprintln(os.Stdout, "usage: hostbridge <command> [args...]")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Commands for ctgbot hostbridge:")
	fmt.Fprintln(os.Stdout, "version - Show embedded hostbridge version")
	router, err := hostbridgeRouter()
	if err != nil {
		fmt.Fprintln(os.Stdout, "help unavailable:", err)
	} else if err := router.FPrintHelpIndex(context.Background(), os.Stdout, actor); err != nil {
		fmt.Fprintln(os.Stdout, "help unavailable:", err)
	}
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
