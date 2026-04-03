package hostbridge

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

type AllowedCommand struct {
	Path      string
	ArgPolicy func([]string) error
}

func Serve(ctx context.Context, address string, defaultTimeoutSec int, allowed map[string]AllowedCommand, logger *log.Logger) error {
	if strings.TrimSpace(address) == "" {
		return fmt.Errorf("missing address")
	}
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	if defaultTimeoutSec <= 0 {
		defaultTimeoutSec = 30
	}
	if allowed == nil {
		allowed = DefaultAllowedCommands()
	}

	ln, err := Listen(address)
	if err != nil {
		return err
	}
	defer ln.Close()

	return ServeListener(ctx, ln, defaultTimeoutSec, allowed, logger)
}

func ServeListener(ctx context.Context, ln net.Listener, defaultTimeoutSec int, allowed map[string]AllowedCommand, logger *log.Logger) error {
	if ln == nil {
		return fmt.Errorf("missing listener")
	}
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	if defaultTimeoutSec <= 0 {
		defaultTimeoutSec = 30
	}
	if allowed == nil {
		allowed = DefaultAllowedCommands()
	}

	logger.Printf("hostbridge controller listening on %s://%s", ln.Addr().Network(), ln.Addr().String())

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

func Listen(address string) (net.Listener, error) {
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", address, err)
	}
	return ln, nil
}

func handleConn(conn net.Conn, allowed map[string]AllowedCommand, defaultTimeoutSec int, logger *log.Logger) {
	defer conn.Close()

	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)
	send := &safeEncoder{enc: enc}

	var req Request
	if err := dec.Decode(&req); err != nil {
		_ = send.Encode(Frame{Kind: StreamError, Message: "decode request: " + err.Error()})
		return
	}

	spec, ok := allowed[req.Command]
	if !ok {
		_ = send.Encode(Frame{Kind: StreamError, Message: "command not allowed: " + req.Command})
		return
	}
	if spec.ArgPolicy != nil {
		if err := spec.ArgPolicy(req.Args); err != nil {
			_ = send.Encode(Frame{Kind: StreamError, Message: "arg validation failed: " + err.Error()})
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
		_ = send.Encode(Frame{Kind: StreamError, Message: "stdin pipe: " + err.Error()})
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = send.Encode(Frame{Kind: StreamError, Message: "stdout pipe: " + err.Error()})
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = send.Encode(Frame{Kind: StreamError, Message: "stderr pipe: " + err.Error()})
		return
	}

	if err := cmd.Start(); err != nil {
		_ = send.Encode(Frame{Kind: StreamError, Message: "start command: " + err.Error()})
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
	go streamReader(send, stdout, StreamStdout, done)
	go streamReader(send, stderr, StreamStderr, done)

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
			_ = send.Encode(Frame{Kind: StreamError, Message: "wait command: " + err.Error()})
			return
		}
	}

	_ = send.Encode(Frame{Kind: StreamExit, ExitCode: exitCode})
}

func streamReader(enc *safeEncoder, r io.Reader, kind StreamKind, done chan<- struct{}) {
	defer func() { done <- struct{}{} }()

	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if encodeErr := enc.Encode(Frame{Kind: kind, Data: chunk}); encodeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func DefaultAllowedCommands() map[string]AllowedCommand {
	allowed := map[string]AllowedCommand{}

	if runtime.GOOS == "windows" {
		allowed["dir"] = AllowedCommand{
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
			allowed[pair.name] = AllowedCommand{Path: pair.path, ArgPolicy: DefaultArgPolicy}
		}
	}

	return allowed
}

func DefaultArgPolicy(args []string) error {
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
