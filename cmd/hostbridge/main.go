package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/chatcommands"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	clientpkg "github.com/bartdeboer/ctgbot/internal/hostbridge/client"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func main() {
	args := normalizedArgs(os.Args[1:])
	if len(args) == 0 || (len(args) == 1 && args[0] == "help") {
		printHelp()
		return
	}

	cmds := chatcommands.New(nil)
	base, err := baseRequest()
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	req, err := cmds.ParseBridge(context.Background(), base, args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		printHelp()
		os.Exit(1)
	}

	resp, err := clientpkg.Do(context.Background(), getenv("HOSTBRIDGE_ADDR", "host.docker.internal:4567"), getenv("HOSTBRIDGE_TLS_DIR", ""), hostbridge.Request{Request: req})
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

func baseRequest() (chatcommands.Request, error) {
	sandboxIDText := strings.TrimSpace(os.Getenv("CTGBOT_SANDBOX_ID"))
	if sandboxIDText == "" {
		return chatcommands.Request{}, nil
	}
	sandboxID, err := modeluuid.Parse(sandboxIDText)
	if err != nil {
		return chatcommands.Request{}, fmt.Errorf("parse CTGBOT_SANDBOX_ID: %w", err)
	}
	return chatcommands.Request{SandboxID: sandboxID}, nil
}

func getenv(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func printHelp() {
	cmds := chatcommands.New(nil)
	fmt.Fprintln(os.Stdout, "usage: hostbridge <command> [args...]")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, cmds.BridgeHelpText())
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "environment:")
	fmt.Fprintln(os.Stdout, "  HOSTBRIDGE_ADDR     TCP address (default host.docker.internal:4567)")
	fmt.Fprintln(os.Stdout, "  HOSTBRIDGE_TLS_DIR  Optional directory containing ca.crt, client.crt, client.key")
	fmt.Fprintln(os.Stdout, "  CTGBOT_SANDBOX_ID   Sandbox/thread id for outbound/config commands")
}
