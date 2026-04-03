package main

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-codextgbot/internal/hostbridge"
)

type allowedCommand struct {
	Path      string
	ArgPolicy func([]string) error
}

func main() {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "help" {
		printHostbridgeControllerHelp()
		return
	}
	if len(args) == 0 {
		args = []string{"serve"}
	}

	r := clir.New()
	r.Routes(func(b *clir.Builder) {
		b.Handle("serve", "Serve the hostbridge controller", func(req *clir.Request) error {
			fs := flag.NewFlagSet("hostbridge-controller serve", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)

			socketPath := fs.String("socket", getenv("HOSTBRIDGE_SOCKET", "/run/hostbridge/bridge.sock"), "Unix socket path")
			timeoutSec := fs.Int("default-timeout-sec", 30, "Default timeout in seconds")
			var allow allowFlag
			fs.Var(&allow, "allow", "Additional allowed command mapping in the form name=/absolute/path")

			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			return serve(*socketPath, *timeoutSec, allow)
		})
	})

	if err := r.Run(context.Background(), args); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		printHostbridgeControllerHelp()
		os.Exit(1)
	}
}

func serve(socketPath string, defaultTimeoutSec int, allow allowFlag) error {
	if err := os.MkdirAll(filepath.Dir(socketPath), 0o750); err != nil {
		return fmt.Errorf("mkdir socket dir: %w", err)
	}
	_ = os.Remove(socketPath)

	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	defer ln.Close()

	if err := os.Chmod(socketPath, 0o660); err != nil {
		return fmt.Errorf("chmod socket: %w", err)
	}

	logger := log.New(os.Stdout, "", log.LstdFlags)
	logger.Printf("hostbridge controller listening on %s", socketPath)

	allowed := defaultAllowedCommands()
	for name, path := range allow.values {
		allowed[name] = allowedCommand{Path: path, ArgPolicy: defaultArgPolicy}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			logger.Printf("accept error: %v", err)
			continue
		}
		go handleConn(conn, allowed, defaultTimeoutSec, logger)
	}
}

func handleConn(conn net.Conn, allowed map[string]allowedCommand, defaultTimeoutSec int, logger *log.Logger) {
	defer conn.Close()

	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)
	send := &safeEncoder{enc: enc}

	var req hostbridge.Request
	if err := dec.Decode(&req); err != nil {
		_ = send.Encode(hostbridge.Frame{Kind: hostbridge.StreamError, Message: "decode request: " + err.Error()})
		return
	}

	spec, ok := allowed[req.Command]
	if !ok {
		_ = send.Encode(hostbridge.Frame{Kind: hostbridge.StreamError, Message: "command not allowed: " + req.Command})
		return
	}
	if spec.ArgPolicy != nil {
		if err := spec.ArgPolicy(req.Args); err != nil {
			_ = send.Encode(hostbridge.Frame{Kind: hostbridge.StreamError, Message: "arg validation failed: " + err.Error()})
			return
		}
	}

	timeout := time.Duration(defaultTimeoutSec) * time.Second
	if req.Timeout > 0 && req.Timeout <= 600 {
		timeout = time.Duration(req.Timeout) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	execPath := spec.Path
	execArgs := req.Args
	if req.Command == "dir" && runtime.GOOS == "windows" {
		execArgs = append([]string{"/c", "dir"}, req.Args...)
	}

	cmd := exec.CommandContext(ctx, execPath, execArgs...)
	if strings.TrimSpace(req.Cwd) != "" {
		cmd.Dir = req.Cwd
	}
	cmd.Env = sanitizedEnv(req.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = send.Encode(hostbridge.Frame{Kind: hostbridge.StreamError, Message: "stdin pipe: " + err.Error()})
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = send.Encode(hostbridge.Frame{Kind: hostbridge.StreamError, Message: "stdout pipe: " + err.Error()})
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = send.Encode(hostbridge.Frame{Kind: hostbridge.StreamError, Message: "stderr pipe: " + err.Error()})
		return
	}

	if err := cmd.Start(); err != nil {
		_ = send.Encode(hostbridge.Frame{Kind: hostbridge.StreamError, Message: "start command: " + err.Error()})
		return
	}

	logger.Printf("hostbridge command=%s args=%q cwd=%q", req.Command, req.Args, req.Cwd)

	go func() {
		defer stdin.Close()
		if len(req.Stdin) > 0 {
			_, _ = io.Copy(stdin, bytes.NewReader(req.Stdin))
		}
	}()

	done := make(chan struct{}, 2)
	go streamReader(send, stdout, hostbridge.StreamStdout, done)
	go streamReader(send, stderr, hostbridge.StreamStderr, done)

	err = cmd.Wait()
	<-done
	<-done

	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.As(err, &exitErr):
			exitCode = exitErr.ExitCode()
		case ctx.Err() == context.DeadlineExceeded:
			exitCode = 124
		default:
			_ = send.Encode(hostbridge.Frame{Kind: hostbridge.StreamError, Message: "wait command: " + err.Error()})
			return
		}
	}

	_ = send.Encode(hostbridge.Frame{Kind: hostbridge.StreamExit, ExitCode: exitCode})
}

func streamReader(enc *safeEncoder, r io.Reader, kind hostbridge.StreamKind, done chan<- struct{}) {
	defer func() { done <- struct{}{} }()

	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if encodeErr := enc.Encode(hostbridge.Frame{Kind: kind, Data: chunk}); encodeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func defaultAllowedCommands() map[string]allowedCommand {
	allowed := map[string]allowedCommand{}

	if runtime.GOOS == "windows" {
		allowed["dir"] = allowedCommand{
			Path: `C:\Windows\System32\cmd.exe`,
			ArgPolicy: func(args []string) error {
				for _, a := range args {
					if strings.ContainsAny(a, "&|;<>") {
						return fmt.Errorf("disallowed metacharacter in arg: %q", a)
					}
				}
				return nil
			},
		}
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
			allowed[pair.name] = allowedCommand{Path: pair.path, ArgPolicy: defaultArgPolicy}
		}
	}

	return allowed
}

func defaultArgPolicy(args []string) error {
	for _, a := range args {
		if strings.ContainsAny(a, ";&|<>") {
			return fmt.Errorf("disallowed metacharacter in arg: %q", a)
		}
	}
	return nil
}

func sanitizedEnv(extra map[string]string) []string {
	base := []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"LANG=C",
	}
	for k, v := range extra {
		if strings.TrimSpace(k) == "" || strings.ContainsRune(k, '=') {
			continue
		}
		base = append(base, k+"="+v)
	}
	return base
}

type safeEncoder struct {
	mu  sync.Mutex
	enc *gob.Encoder
}

func (s *safeEncoder) Encode(v any) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enc.Encode(v)
}

type allowFlag struct {
	values map[string]string
}

func (f *allowFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	var parts []string
	for k, v := range f.values {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func (f *allowFlag) Set(v string) error {
	if f.values == nil {
		f.values = map[string]string{}
	}
	name, path, ok := strings.Cut(v, "=")
	if !ok {
		return fmt.Errorf("expected name=/absolute/path")
	}
	name = strings.TrimSpace(name)
	path = strings.TrimSpace(path)
	if name == "" || path == "" {
		return fmt.Errorf("expected name=/absolute/path")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute: %s", path)
	}
	f.values[name] = path
	return nil
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func printHostbridgeControllerHelp() {
	fmt.Fprintln(os.Stdout, "usage: hostbridge-controller serve [--socket PATH] [--allow name=/absolute/path]")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "examples:")
	fmt.Fprintln(os.Stdout, "  hostbridge-controller serve")
	fmt.Fprintln(os.Stdout, "  hostbridge-controller serve --allow ls=/bin/ls")
}
