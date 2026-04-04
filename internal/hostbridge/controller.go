package hostbridge

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/gob"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type AllowedCommand struct {
	Path string
}

type securityTaggedListener struct {
	net.Listener
	securityMode string
}

type AllowedCommandResolver func(clientIdentity string) map[string]AllowedCommand

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

	return ServeListener(ctx, ln, defaultTimeoutSec, StaticAllowedCommandResolver(allowed), logger)
}

func ServeListener(ctx context.Context, ln net.Listener, defaultTimeoutSec int, resolve AllowedCommandResolver, logger *log.Logger) error {
	if ln == nil {
		return fmt.Errorf("missing listener")
	}
	if logger == nil {
		logger = log.New(io.Discard, "", 0)
	}
	if defaultTimeoutSec <= 0 {
		defaultTimeoutSec = 30
	}
	if resolve == nil {
		resolve = StaticAllowedCommandResolver(nil)
	}

	logger.Printf("hostbridge controller listening on %s://%s security=%s", ln.Addr().Network(), ln.Addr().String(), listenerSecurityMode(ln))

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
		go handleConn(conn, resolve, defaultTimeoutSec, logger)
	}
}

func Listen(address string) (net.Listener, error) {
	// Keep hostbridge physically local to the machine. This controller is a
	// privileged host-command bridge for containers, so binding it anywhere other
	// than the host loopback interface would expose host command execution beyond
	// the local machine.
	if err := validateLoopbackListenAddress(address); err != nil {
		return nil, err
	}
	ln, err := net.Listen("tcp", address)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", address, err)
	}
	return &securityTaggedListener{Listener: ln, securityMode: "plain-tcp"}, nil
}

func ListenTLS(address string, tlsConfig *tls.Config) (net.Listener, error) {
	if err := validateLoopbackListenAddress(address); err != nil {
		return nil, err
	}
	if tlsConfig == nil {
		return nil, fmt.Errorf("missing tls config")
	}
	ln, err := tls.Listen("tcp", address, tlsConfig)
	if err != nil {
		return nil, fmt.Errorf("listen tls on %s: %w", address, err)
	}
	return &securityTaggedListener{Listener: ln, securityMode: "tls-mtls"}, nil
}

func validateLoopbackListenAddress(address string) error {
	host, _, err := net.SplitHostPort(strings.TrimSpace(address))
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", address, err)
	}
	if host != "127.0.0.1" {
		return fmt.Errorf("hostbridge must listen on 127.0.0.1 only, got %q", host)
	}
	return nil
}

func handleConn(conn net.Conn, resolve AllowedCommandResolver, defaultTimeoutSec int, logger *log.Logger) {
	defer conn.Close()

	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)
	send := &safeEncoder{enc: enc}

	var req Request
	if err := dec.Decode(&req); err != nil {
		_ = send.Encode(Frame{Kind: StreamError, Message: "decode request: " + err.Error()})
		return
	}

	clientIdentity := connectionClientIdentity(conn)
	allowed := resolve(clientIdentity)
	if allowed == nil {
		allowed = DefaultAllowedCommands()
	}

	spec, ok := allowed[req.Command]
	if !ok {
		_ = send.Encode(Frame{Kind: StreamError, Message: "command not allowed: " + req.Command})
		return
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

	logger.Printf("hostbridge command=%s args=%q cwd=%q security=%s client=%q", req.Command, req.Args, req.Cwd, connectionSecurityMode(conn), clientIdentity)

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
			allowed[pair.name] = AllowedCommand{Path: pair.path}
		}
	}

	return allowed
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

func MergeAllowedCommands(extra map[string]string) map[string]AllowedCommand {
	allowed := DefaultAllowedCommands()
	for name, path := range extra {
		name = strings.TrimSpace(name)
		path = strings.TrimSpace(path)
		if name == "" || path == "" {
			continue
		}
		allowed[name] = AllowedCommand{Path: path}
	}
	return allowed
}

func MergeAllowedCommandSpecs(specs []string) map[string]AllowedCommand {
	allowed := DefaultAllowedCommands()
	for name, spec := range AllowedCommandsFromSpecs(specs) {
		allowed[name] = spec
	}
	return allowed
}

func AllowedCommandsFromSpecs(specs []string) map[string]AllowedCommand {
	allowed := map[string]AllowedCommand{}
	for _, spec := range specs {
		spec = strings.TrimSpace(spec)
		if spec == "" {
			continue
		}
		name := filepath.Base(spec)
		name = strings.TrimSpace(name)
		if name == "" || name == "." || name == string(filepath.Separator) {
			continue
		}
		allowed[name] = AllowedCommand{Path: spec}
	}
	return allowed
}

func AllowedCommandNames(allowed map[string]AllowedCommand) []string {
	if len(allowed) == 0 {
		return nil
	}
	names := make([]string, 0, len(allowed))
	for name := range allowed {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func StaticAllowedCommandResolver(allowed map[string]AllowedCommand) AllowedCommandResolver {
	if allowed == nil {
		allowed = DefaultAllowedCommands()
	}
	return func(string) map[string]AllowedCommand {
		return allowed
	}
}

func listenerSecurityMode(ln net.Listener) string {
	if tagged, ok := ln.(*securityTaggedListener); ok {
		return tagged.securityMode
	}
	return "unknown"
}

func connectionSecurityMode(conn net.Conn) string {
	if _, ok := conn.(*tls.Conn); ok {
		return "tls-mtls"
	}
	return "plain-tcp"
}

func connectionClientIdentity(conn net.Conn) string {
	tlsConn, ok := conn.(*tls.Conn)
	if !ok {
		return ""
	}
	state := tlsConn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		return ""
	}
	return state.PeerCertificates[0].Subject.CommonName
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
