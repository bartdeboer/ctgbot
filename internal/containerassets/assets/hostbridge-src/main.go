package main

import (
	"encoding/gob"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
)

type Request struct {
	Command string
	Args    []string
	Stdin   []byte
	Cwd     string
	Env     map[string]string
	Timeout int
}

type StreamKind uint8

const (
	StreamStdout StreamKind = 1
	StreamStderr StreamKind = 2
	StreamExit   StreamKind = 3
	StreamError  StreamKind = 4
)

type Frame struct {
	Kind     StreamKind
	Data     []byte
	ExitCode int
	Message  string
}

func main() {
	args := os.Args[1:]
	if len(args) == 0 || args[0] == "help" || args[0] == "--help" || args[0] == "-h" {
		printHelp()
		return
	}

	socketPath := getenv("HOSTBRIDGE_SOCKET", "/run/hostbridge/bridge.sock")
	if env := os.Getenv("HOSTBRIDGE_TIMEOUT_SEC"); strings.TrimSpace(env) != "" {
		_ = env
	}

	stdinData, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "read stdin: %v\n", err)
		os.Exit(1)
	}

	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "connect socket %s: %v\n", socketPath, err)
		os.Exit(1)
	}
	defer conn.Close()

	enc := gob.NewEncoder(conn)
	dec := gob.NewDecoder(conn)

	req := Request{
		Command: args[0],
		Args:    args[1:],
		Stdin:   stdinData,
		Timeout: 30,
	}

	if err := enc.Encode(req); err != nil {
		fmt.Fprintf(os.Stderr, "send request: %v\n", err)
		os.Exit(1)
	}

	for {
		var frame Frame
		if err := dec.Decode(&frame); err != nil {
			fmt.Fprintf(os.Stderr, "read response: %v\n", err)
			os.Exit(1)
		}
		switch frame.Kind {
		case StreamStdout:
			_, _ = os.Stdout.Write(frame.Data)
		case StreamStderr:
			_, _ = os.Stderr.Write(frame.Data)
		case StreamError:
			fmt.Fprintln(os.Stderr, frame.Message)
			os.Exit(1)
		case StreamExit:
			os.Exit(frame.ExitCode)
		default:
			fmt.Fprintf(os.Stderr, "unknown frame kind: %d\n", frame.Kind)
			os.Exit(1)
		}
	}
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func printHelp() {
	fmt.Fprintln(os.Stdout, "usage: hostbridge <command> [args...]")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "examples:")
	fmt.Fprintln(os.Stdout, "  hostbridge ls -la")
	fmt.Fprintln(os.Stdout, "  hostbridge pwd")
}
