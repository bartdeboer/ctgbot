package main

import (
	"context"
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"os"
	"strings"

	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-codextgbot/internal/hostbridge"
)

func main() {
	rawArgs := os.Args[1:]
	args := rawArgs
	if len(args) == 0 || (len(args) == 1 && args[0] == "help") {
		printHostbridgeHelp()
		return
	}
	if len(args) > 0 && !isHostbridgeManagementCommand(args[0]) {
		args = append([]string{"run"}, args...)
	}

	r := clir.New()
	r.Routes(func(b *clir.Builder) {
		b.Handle("run <command>", "Run a whitelisted host command over the bridge socket", func(req *clir.Request) error {
			stdinData, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}

			network := "unix"
			address := getenv("HOSTBRIDGE_SOCKET", "/run/hostbridge/bridge.sock")
			if tcpAddr := getenv("HOSTBRIDGE_ADDR", ""); tcpAddr != "" {
				network = "tcp"
				address = tcpAddr
			}

			conn, err := net.Dial(network, address)
			if err != nil {
				return fmt.Errorf("connect %s %s: %w", network, address, err)
			}
			defer conn.Close()

			enc := gob.NewEncoder(conn)
			dec := gob.NewDecoder(conn)

			payload := hostbridge.Request{
				Command: req.Params["command"],
				Args:    req.Extra,
				Stdin:   stdinData,
				Timeout: 30,
			}

			if err := enc.Encode(payload); err != nil {
				return fmt.Errorf("send request: %w", err)
			}

			for {
				var frame hostbridge.Frame
				if err := dec.Decode(&frame); err != nil {
					return fmt.Errorf("read response: %w", err)
				}
				switch frame.Kind {
				case hostbridge.StreamStdout:
					if _, err := os.Stdout.Write(frame.Data); err != nil {
						return err
					}
				case hostbridge.StreamStderr:
					if _, err := os.Stderr.Write(frame.Data); err != nil {
						return err
					}
				case hostbridge.StreamError:
					return fmt.Errorf(frame.Message)
				case hostbridge.StreamExit:
					if frame.ExitCode != 0 {
						os.Exit(frame.ExitCode)
					}
					return nil
				default:
					return fmt.Errorf("unknown frame kind: %d", frame.Kind)
				}
			}
		})
	})

	if err := r.Run(context.Background(), args); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		printHostbridgeHelp()
		os.Exit(1)
	}
}

func isHostbridgeManagementCommand(arg string) bool {
	switch arg {
	case "", "run":
		return true
	default:
		return false
	}
}

func printHostbridgeHelp() {
	fmt.Fprintln(os.Stdout, "usage: hostbridge <command> [args...]")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "environment:")
	fmt.Fprintln(os.Stdout, "  HOSTBRIDGE_ADDR     TCP address (for example host.docker.internal:4567)")
	fmt.Fprintln(os.Stdout, "  HOSTBRIDGE_SOCKET   Unix socket path (default /run/hostbridge/bridge.sock)")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "examples:")
	fmt.Fprintln(os.Stdout, "  hostbridge ls -la")
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
