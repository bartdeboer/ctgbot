package main

import (
	"context"
	"encoding/gob"
	"flag"
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
			fs := flag.NewFlagSet("hostbridge run", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)

			socketPath := fs.String("socket", getenv("HOSTBRIDGE_SOCKET", "/run/hostbridge/bridge.sock"), "Unix socket path")
			timeoutSec := fs.Int("timeout-sec", 30, "Command timeout in seconds")
			cwd := fs.String("cwd", "", "Optional working directory on the host")

			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			stdinData, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}

			conn, err := net.Dial("unix", *socketPath)
			if err != nil {
				return fmt.Errorf("connect socket %s: %w", *socketPath, err)
			}
			defer conn.Close()

			enc := gob.NewEncoder(conn)
			dec := gob.NewDecoder(conn)

			payload := hostbridge.Request{
				Command: req.Params["command"],
				Args:    fs.Args(),
				Stdin:   stdinData,
				Cwd:     strings.TrimSpace(*cwd),
				Timeout: *timeoutSec,
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
	fmt.Fprintln(os.Stdout, "examples:")
	fmt.Fprintln(os.Stdout, "  hostbridge ls -la")
	fmt.Fprintln(os.Stdout, "  hostbridge run --socket /run/hostbridge/bridge.sock ls -la")
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
