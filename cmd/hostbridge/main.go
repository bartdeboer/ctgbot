package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	clientpkg "github.com/bartdeboer/ctgbot/internal/hostbridge/client"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/schema/routers"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func main() {
	args := normalizedArgs(os.Args[1:])
	if len(args) == 0 || (len(args) == 1 && args[0] == "help") {
		printHelp()
		return
	}

	base, err := baseRequest()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	router, err := routers.NewHostbridgeRouter()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	req, err := router.Parse(context.Background(), base, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		printHelp()
		os.Exit(1)
	}

	resp, err := clientpkg.DoCommand(context.Background(), getenv("HOSTBRIDGE_ADDR", "host.docker.internal:4567"), getenv("HOSTBRIDGE_TLS_DIR", ""), hostbridge.CommandRequest{Request: req})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if strings.TrimSpace(resp.Result.Text) != "" {
		fmt.Fprintln(os.Stdout, resp.Result.Text)
	}
}

func normalizedArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	if len(args) >= 2 && args[0] == "run" && args[1] == "sendstdin" {
		return append([]string{"sendstdin"}, args[2:]...)
	}
	if isManagementCommand(args[0]) {
		return args
	}
	return append([]string{"run"}, args...)
}

func isManagementCommand(arg string) bool {
	switch arg {
	case "", "run", "sendfile", "sendstdin", "config", "refresh", "purge", "interrupt", "upgrade", "quit", "stop", "status", "container", "chat", "help":
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
	fmt.Fprintln(os.Stdout, "Commands for Telegram-attached ctgbot hostbridge:")
	printDefinitionHelp(routers.HostbridgeDefinitions())
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "Standalone ctgbot hostbridge serve accepts only:")
	printDefinitionHelp(routers.HostbridgeRunDefinitions())
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "environment:")
	fmt.Fprintln(os.Stdout, "  HOSTBRIDGE_ADDR     TCP address (default host.docker.internal:4567)")
	fmt.Fprintln(os.Stdout, "  HOSTBRIDGE_TLS_DIR  Optional directory containing ca.crt, client.crt, client.key")
	fmt.Fprintln(os.Stdout, "  CTGBOT_SANDBOX_ID   Sandbox/thread id for outbound/config commands")
}

func printDefinitionHelp(definitions []commandengine.Definition) {
	for _, definition := range definitions {
		for _, route := range definition.Routes {
			if route.Help == "" {
				fmt.Fprintf(os.Stdout, "%s\n", route.Pattern)
				continue
			}
			fmt.Fprintf(os.Stdout, "%s - %s\n", route.Pattern, route.Help)
		}
	}
}
