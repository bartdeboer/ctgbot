package server

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

	"github.com/bartdeboer/ctgbot/internal/durationparse"
	hbprotocol "github.com/bartdeboer/ctgbot/internal/hostbridge/protocol"
)

type AllowedCommand struct {
	Name           string
	Args           []string
	Dir            string
	Delay          string
	Env            map[string]string
	AllowExtraArgs bool
}

type securityTaggedListener struct {
	net.Listener
	securityMode string
}

type AllowedCommandResolver func(clientIdentity string) map[string]AllowedCommand

type SendFileRequest struct {
	SandboxID   string
	Filename    string
	Caption     string
	ContentType string
	Content     []byte
}

type SendFileHandler func(ctx context.Context, req SendFileRequest) error

type SendTextRequest struct {
	SandboxID   string
	Text        string
	ContentType string
}

type SendTextHandler func(ctx context.Context, req SendTextRequest) error

type ConfigListRequest struct {
	SandboxID string
}

type ConfigListHandler func(ctx context.Context, req ConfigListRequest) (string, error)

type ConfigSetRequest struct {
	SandboxID string
	Setting   string
	Value     string
}

type ConfigSetHandler func(ctx context.Context, req ConfigSetRequest) (string, error)

type tlsListenerConfig interface {
	HostbridgeTCPListenAddr() string
}

func Serve(ctx context.Context, address string, defaultTimeoutSec int, allowed map[string]AllowedCommand, sendFile SendFileHandler, sendText SendTextHandler, configList ConfigListHandler, configSet ConfigSetHandler, logger *log.Logger) error {
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

	return ServeListener(ctx, ln, defaultTimeoutSec, StaticAllowedCommandResolver(allowed), sendFile, sendText, configList, configSet, logger)
}

func ServeListener(ctx context.Context, ln net.Listener, defaultTimeoutSec int, resolve AllowedCommandResolver, sendFile SendFileHandler, sendText SendTextHandler, configList ConfigListHandler, configSet ConfigSetHandler, logger *log.Logger) error {
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
		go HandleConn(conn, resolve, sendFile, sendText, configList, configSet, defaultTimeoutSec, logger)
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

func NewTLSListener(cfg tlsListenerConfig, tlsConfig *tls.Config) (net.Listener, error) {
	if cfg == nil {
		return nil, fmt.Errorf("missing config")
	}
	return ListenTLS(cfg.HostbridgeTCPListenAddr(), tlsConfig)
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

func HandleConn(conn net.Conn, resolve AllowedCommandResolver, sendFile SendFileHandler, sendText SendTextHandler, configList ConfigListHandler, configSet ConfigSetHandler, defaultTimeoutSec int, logger *log.Logger) {
	defer conn.Close()

	dec := gob.NewDecoder(conn)
	enc := gob.NewEncoder(conn)
	send := &safeEncoder{enc: enc}

	var req hbprotocol.Request
	if err := dec.Decode(&req); err != nil {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "decode request: " + err.Error()})
		return
	}

	switch req.Op {
	case "", hbprotocol.OpRunCommand:
		handleRunCommand(conn, send, req, resolve, defaultTimeoutSec, logger)
	case hbprotocol.OpSendFile:
		handleSendFile(conn, send, req, sendFile, defaultTimeoutSec, logger)
	case hbprotocol.OpSendText:
		handleSendText(conn, send, req, sendText, defaultTimeoutSec, logger)
	case hbprotocol.OpConfigList:
		handleConfigList(conn, send, req, configList, defaultTimeoutSec, logger)
	case hbprotocol.OpConfigSet:
		handleConfigSet(conn, send, req, configSet, defaultTimeoutSec, logger)
	default:
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "unsupported operation: " + string(req.Op)})
	}
}

func handleRunCommand(conn net.Conn, send *safeEncoder, req hbprotocol.Request, resolve AllowedCommandResolver, defaultTimeoutSec int, logger *log.Logger) {
	clientIdentity := connectionClientIdentity(conn)
	allowed := resolve(clientIdentity)
	if allowed == nil {
		allowed = DefaultAllowedCommands()
	}

	spec, ok := allowed[req.Command]
	if !ok {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "command not allowed: " + req.Command})
		return
	}

	timeout := time.Duration(defaultTimeoutSec) * time.Second
	if req.Timeout > 0 && req.Timeout <= 600 {
		timeout = time.Duration(req.Timeout) * time.Second
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	plan, err := BuildExecutionPlan(req, spec)
	if err != nil {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: err.Error()})
		return
	}

	cmd := exec.CommandContext(ctx, plan.Name, plan.Args...)
	cmd.Dir = plan.Dir
	cmd.Env = plan.Env
	if plan.Delay > 0 {
		timer := time.NewTimer(plan.Delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: ctx.Err().Error()})
			return
		}
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "stdin pipe: " + err.Error()})
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "stdout pipe: " + err.Error()})
		return
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "stderr pipe: " + err.Error()})
		return
	}

	if err := cmd.Start(); err != nil {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "start command: " + err.Error()})
		return
	}

	logger.Printf("hostbridge command=%s args=%q dir=%q security=%s client=%q", req.Command, plan.Args, plan.Dir, connectionSecurityMode(conn), clientIdentity)

	go func() {
		defer stdin.Close()
		if len(req.Stdin) > 0 {
			_, _ = io.Copy(stdin, bytes.NewReader(req.Stdin))
		}
	}()

	done := make(chan struct{}, 2)
	go streamReader(send, stdout, hbprotocol.StreamStdout, done)
	go streamReader(send, stderr, hbprotocol.StreamStderr, done)

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
			_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "wait command: " + err.Error()})
			return
		}
	}

	_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamExit, ExitCode: exitCode})
}

func handleSendFile(conn net.Conn, send *safeEncoder, req hbprotocol.Request, sendFile SendFileHandler, defaultTimeoutSec int, logger *log.Logger) {
	if sendFile == nil {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "sendfile not configured"})
		return
	}
	if strings.TrimSpace(req.SandboxID) == "" {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "missing sandbox id"})
		return
	}
	if strings.TrimSpace(req.Filename) == "" {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "missing filename"})
		return
	}
	if len(req.Content) > hbprotocol.MaxSendFileBytes {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: fmt.Sprintf("file exceeds %d byte limit", hbprotocol.MaxSendFileBytes)})
		return
	}

	timeout := time.Duration(defaultTimeoutSec) * time.Second
	if req.Timeout > 0 && req.Timeout <= 600 {
		timeout = time.Duration(req.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	logger.Printf("hostbridge sendfile filename=%q bytes=%d security=%s client=%q sandbox=%q", req.Filename, len(req.Content), connectionSecurityMode(conn), connectionClientIdentity(conn), req.SandboxID)

	err := sendFile(ctx, SendFileRequest{
		SandboxID:   req.SandboxID,
		Filename:    req.Filename,
		Caption:     req.Caption,
		ContentType: req.ContentType,
		Content:     req.Content,
	})
	if err != nil {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: err.Error()})
		return
	}
	_ = send.Encode(hbprotocol.Frame{
		Kind: hbprotocol.StreamStdout,
		Data: []byte(fmt.Sprintf("sent file: %s\n", req.Filename)),
	})
	_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamExit, ExitCode: 0})
}

func handleConfigList(conn net.Conn, send *safeEncoder, req hbprotocol.Request, configList ConfigListHandler, defaultTimeoutSec int, logger *log.Logger) {
	if configList == nil {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "config list not configured"})
		return
	}
	if strings.TrimSpace(req.SandboxID) == "" {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "missing sandbox id"})
		return
	}
	timeout := time.Duration(defaultTimeoutSec) * time.Second
	if req.Timeout > 0 && req.Timeout <= 600 {
		timeout = time.Duration(req.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	logger.Printf("hostbridge config list security=%s client=%q sandbox=%q", connectionSecurityMode(conn), connectionClientIdentity(conn), req.SandboxID)
	out, err := configList(ctx, ConfigListRequest{SandboxID: req.SandboxID})
	if err != nil {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: err.Error()})
		return
	}
	if strings.TrimSpace(out) != "" {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamStdout, Data: []byte(out + "\n")})
	}
	_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamExit, ExitCode: 0})
}

func handleConfigSet(conn net.Conn, send *safeEncoder, req hbprotocol.Request, configSet ConfigSetHandler, defaultTimeoutSec int, logger *log.Logger) {
	if configSet == nil {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "config set not configured"})
		return
	}
	if strings.TrimSpace(req.SandboxID) == "" {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "missing sandbox id"})
		return
	}
	if strings.TrimSpace(req.Setting) == "" {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "missing setting"})
		return
	}
	timeout := time.Duration(defaultTimeoutSec) * time.Second
	if req.Timeout > 0 && req.Timeout <= 600 {
		timeout = time.Duration(req.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	logger.Printf("hostbridge config set setting=%q security=%s client=%q sandbox=%q", req.Setting, connectionSecurityMode(conn), connectionClientIdentity(conn), req.SandboxID)
	out, err := configSet(ctx, ConfigSetRequest{SandboxID: req.SandboxID, Setting: req.Setting, Value: req.Value})
	if err != nil {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: err.Error()})
		return
	}
	if strings.TrimSpace(out) != "" {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamStdout, Data: []byte(out + "\n")})
	}
	_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamExit, ExitCode: 0})
}

func streamReader(enc *safeEncoder, r io.Reader, kind hbprotocol.StreamKind, done chan<- struct{}) {
	defer func() { done <- struct{}{} }()

	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])
			if encodeErr := enc.Encode(hbprotocol.Frame{Kind: kind, Data: chunk}); encodeErr != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

type ExecutionPlan struct {
	Name  string
	Args  []string
	Dir   string
	Delay time.Duration
	Env   []string
}

func BuildExecutionPlan(req hbprotocol.Request, spec AllowedCommand) (ExecutionPlan, error) {
	spec, ok := normalizeAllowedCommand(spec)
	if !ok {
		return ExecutionPlan{}, fmt.Errorf("allowed command %q has empty executable name", req.Command)
	}
	delay, err := parseAllowedCommandDelay(req.Command, spec.Delay)
	if err != nil {
		return ExecutionPlan{}, err
	}

	args := append([]string{}, spec.Args...)
	if len(req.Args) > 0 {
		if !spec.AllowExtraArgs {
			return ExecutionPlan{}, fmt.Errorf("command does not allow extra args: %s", req.Command)
		}
		args = append(args, req.Args...)
	}

	return ExecutionPlan{
		Name:  spec.Name,
		Args:  args,
		Dir:   spec.Dir,
		Delay: delay,
		Env:   sanitizedEnv(spec.Env),
	}, nil
}

func DefaultAllowedCommands() map[string]AllowedCommand {
	allowed := map[string]AllowedCommand{}

	if runtime.GOOS == "windows" {
		allowed["dir"] = AllowedCommand{
			Name:           `C:\Windows\System32\cmd.exe`,
			Args:           []string{"/c", "dir"},
			AllowExtraArgs: true,
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
			allowed[pair.name] = AllowedCommand{Name: pair.path, AllowExtraArgs: true}
		}
	}

	return allowed
}

func sanitizedEnv(extra map[string]string) []string {
	base := append([]string{}, os.Environ()...)
	for k, v := range extra {
		if strings.TrimSpace(k) == "" || strings.ContainsRune(k, '=') {
			continue
		}
		base = upsertEnv(base, k, v)
	}
	return base
}

func MergeAllowedCommands(extra map[string]string) map[string]AllowedCommand {
	allowed := DefaultAllowedCommands()
	for name, executable := range extra {
		name = strings.TrimSpace(name)
		executable = strings.TrimSpace(executable)
		if name == "" || executable == "" {
			continue
		}
		allowed[name] = AllowedCommand{Name: executable}
	}
	return allowed
}

func MergeNamedAllowedCommands(extra map[string]AllowedCommand) map[string]AllowedCommand {
	allowed := DefaultAllowedCommands()
	for name, spec := range extra {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if normalized, ok := normalizeAllowedCommand(spec); ok {
			allowed[name] = normalized
		}
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
		allowed[name] = AllowedCommand{Name: spec}
	}
	return allowed
}

func normalizeAllowedCommand(spec AllowedCommand) (AllowedCommand, bool) {
	spec.Name = strings.TrimSpace(spec.Name)
	spec.Dir = strings.TrimSpace(spec.Dir)
	spec.Delay = strings.TrimSpace(spec.Delay)
	spec.Args = cleanCommandArgs(spec.Args)
	spec.Env = cleanCommandEnv(spec.Env)
	if spec.Name == "" {
		return AllowedCommand{}, false
	}
	return spec, true
}

func cleanCommandArgs(args []string) []string {
	if len(args) == 0 {
		return nil
	}
	out := make([]string, 0, len(args))
	for _, arg := range args {
		out = append(out, arg)
	}
	return out
}

func cleanCommandEnv(env map[string]string) map[string]string {
	if len(env) == 0 {
		return nil
	}
	out := make(map[string]string, len(env))
	for key, value := range env {
		key = strings.TrimSpace(key)
		if key == "" || strings.ContainsRune(key, '=') {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func upsertEnv(env []string, key string, value string) []string {
	prefix := key + "="
	for i, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
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

func handleSendText(conn net.Conn, send *safeEncoder, req hbprotocol.Request, sendText SendTextHandler, defaultTimeoutSec int, logger *log.Logger) {
	if sendText == nil {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "sendtext not configured"})
		return
	}
	if strings.TrimSpace(req.SandboxID) == "" {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "missing sandbox id"})
		return
	}
	if req.Text == "" {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: "missing text"})
		return
	}

	timeout := time.Duration(defaultTimeoutSec) * time.Second
	if req.Timeout > 0 && req.Timeout <= 600 {
		timeout = time.Duration(req.Timeout) * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	logger.Printf("hostbridge sendtext bytes=%d fenced=%t language=%q security=%s client=%q sandbox=%q", len(req.Text), req.Fenced, req.Language, connectionSecurityMode(conn), connectionClientIdentity(conn), req.SandboxID)

	err := sendText(ctx, SendTextRequest{
		SandboxID:   req.SandboxID,
		Text:        req.Text,
		ContentType: req.ContentType,
	})
	if err != nil {
		_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamError, Message: err.Error()})
		return
	}
	_ = send.Encode(hbprotocol.Frame{
		Kind: hbprotocol.StreamStdout,
		Data: []byte("sent text\n"),
	})
	_ = send.Encode(hbprotocol.Frame{Kind: hbprotocol.StreamExit, ExitCode: 0})
}

func parseAllowedCommandDelay(commandName string, raw string) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	d, err := durationparse.Parse(raw, time.Millisecond)
	if err != nil {
		return 0, fmt.Errorf("invalid delay %q for command %s: %w", raw, commandName, err)
	}
	if d < 0 {
		return 0, fmt.Errorf("invalid delay %q for command %s: must be >= 0", raw, commandName)
	}
	return d, nil
}
