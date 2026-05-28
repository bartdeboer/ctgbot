package toolloop

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	defaultYieldWait       = 100 * time.Millisecond
	maxExecSessions        = 4
	maxSessionOutputBytes  = 64 * 1024
	charsPerRequestedToken = 4
)

// TODO(toolloop): switch cappedOutput from a tail-only buffer to a Codex-style
// head/tail buffer. Keeping the beginning and end is more useful for commands
// that print setup context first and failures last.
// TODO(toolloop): decide whether exec sessions should remain turn-scoped or
// become conversation/job-scoped with explicit persistence and cleanup policy.
// TODO(toolloop): consider returning structured shell metadata in addition to
// the current model-readable text format when the surrounding protocol supports
// typed tool results.

type ExecSessionManager struct {
	Workspace      string
	CommandTimeout time.Duration
	Sessions       map[string]*ExecSession
	nextID         int
}

type ExecSession struct {
	ID        string
	Command   string
	Cmd       *exec.Cmd
	Stdin     io.WriteCloser
	Output    *cappedOutput
	StartedAt time.Time
	ExitedAt  *time.Time
	ExitCode  *int

	done chan struct{}
	mu   sync.Mutex
}

type writeStdinArgs struct {
	SessionID       string `json:"session_id"`
	Chars           string `json:"chars,omitempty"`
	YieldTimeMS     int    `json:"yield_time_ms,omitempty"`
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
}

type writeStdinLineArgs struct {
	SessionID       string `json:"session_id"`
	Line            string `json:"line"`
	YieldTimeMS     int    `json:"yield_time_ms,omitempty"`
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
}

type shellStopArgs struct {
	SessionID       string `json:"session_id"`
	MaxOutputTokens int    `json:"max_output_tokens,omitempty"`
}

func NewExecSessionManager(workspace string, commandTimeout time.Duration) *ExecSessionManager {
	return &ExecSessionManager{
		Workspace:      workspace,
		CommandTimeout: commandTimeout,
		Sessions:       map[string]*ExecSession{},
	}
}

func (m *ExecSessionManager) Exec(ctx context.Context, args shellArgs) (string, bool) {
	command := strings.TrimSpace(firstNonEmpty(args.Command, args.Cmd))
	if command == "" {
		return "missing shell command", true
	}
	if len(m.Sessions) >= maxExecSessions {
		return fmt.Sprintf("too many active shell sessions (max %d)", maxExecSessions), true
	}
	session, err := m.start(ctx, command, args.Workdir)
	if err != nil {
		return err.Error(), true
	}
	m.Sessions[session.ID] = session

	if args.YieldTimeMS > 0 {
		session.waitFor(time.Duration(args.YieldTimeMS) * time.Millisecond)
		output := session.Output.Drain(m.maxOutputChars(args.MaxOutputTokens))
		if session.exited() {
			delete(m.Sessions, session.ID)
			return session.format(output, false), session.exitCode() != 0
		}
		return session.format(output, true), false
	}

	timedOut := !session.waitUntilDone(m.commandTimeout(args.TimeoutMS))
	output := session.Output.Drain(m.maxOutputChars(args.MaxOutputTokens))
	delete(m.Sessions, session.ID)
	if timedOut {
		session.kill()
		<-session.done
		return session.format(output, false) + "\nerror: shell command timed out", true
	}
	return session.format(output, false), session.exitCode() != 0
}

func (m *ExecSessionManager) WriteStdin(ctx context.Context, args writeStdinArgs) (string, bool) {
	session, err := m.session(args.SessionID)
	if err != nil {
		return err.Error(), true
	}
	if args.Chars != "" {
		if session.exited() {
			output := session.Output.Drain(m.maxOutputChars(args.MaxOutputTokens))
			return session.format(output, true) + "\nerror: session already exited", true
		}
		if _, err := io.WriteString(session.Stdin, args.Chars); err != nil {
			output := session.Output.Drain(m.maxOutputChars(args.MaxOutputTokens))
			return session.format(output, true) + "\nerror: write stdin: " + err.Error(), true
		}
	}
	session.waitFor(m.yieldWait(args.YieldTimeMS))
	output := session.Output.Drain(m.maxOutputChars(args.MaxOutputTokens))
	return session.format(output, true), false
}

func (m *ExecSessionManager) WriteStdinLine(ctx context.Context, args writeStdinLineArgs) (string, bool) {
	return m.WriteStdin(ctx, writeStdinArgs{
		SessionID:       args.SessionID,
		Chars:           args.Line + "\n",
		YieldTimeMS:     args.YieldTimeMS,
		MaxOutputTokens: args.MaxOutputTokens,
	})
}

func (m *ExecSessionManager) Stop(_ context.Context, args shellStopArgs) (string, bool) {
	session, err := m.session(args.SessionID)
	if err != nil {
		return err.Error(), true
	}
	if !session.exited() {
		session.kill()
	}
	<-session.done
	delete(m.Sessions, session.ID)
	output := session.Output.Drain(m.maxOutputChars(args.MaxOutputTokens))
	return session.format(output, true), false
}

func (m *ExecSessionManager) Cleanup() {
	for id, session := range m.Sessions {
		if !session.exited() {
			session.kill()
		}
		<-session.done
		delete(m.Sessions, id)
	}
}

func (m *ExecSessionManager) start(ctx context.Context, command string, requestedWorkdir string) (*ExecSession, error) {
	workspace := firstNonEmpty(m.Workspace, getenv("TOOLLOOP_WORKSPACE"), "/workspace")
	workdir, err := resolveWorkdir(workspace, requestedWorkdir)
	if err != nil {
		return nil, err
	}
	m.nextID++
	id := "session-" + strconv.Itoa(m.nextID)
	cmd := exec.CommandContext(ctx, "/bin/bash", "-c", command)
	cmd.Dir = workdir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	// TODO(toolloop): implement tty=true by allocating a PTY and attaching the
	// command to the PTY slave. The model-facing schema already accepts tty for
	// Codex compatibility, but v1 intentionally uses normal stdin/stdout/stderr
	// pipes for all sessions.

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("open stdin: %w", err)
	}
	session := &ExecSession{
		ID:        id,
		Command:   command,
		Cmd:       cmd,
		Stdin:     stdin,
		Output:    newCappedOutput(maxSessionOutputBytes),
		StartedAt: time.Now().UTC(),
		done:      make(chan struct{}),
	}
	cmd.Stdout = session.Output
	cmd.Stderr = session.Output
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start shell command: %w", err)
	}
	go session.wait()
	return session, nil
}

func (m *ExecSessionManager) session(id string) (*ExecSession, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("missing session_id")
	}
	session, ok := m.Sessions[id]
	if !ok {
		return nil, fmt.Errorf("unknown session_id %q", id)
	}
	return session, nil
}

func (m *ExecSessionManager) commandTimeout(timeoutMS int) time.Duration {
	if timeoutMS > 0 {
		return time.Duration(timeoutMS) * time.Millisecond
	}
	if m.CommandTimeout > 0 {
		return m.CommandTimeout
	}
	return 2 * time.Minute
}

func (m *ExecSessionManager) yieldWait(timeoutMS int) time.Duration {
	if timeoutMS > 0 {
		return time.Duration(timeoutMS) * time.Millisecond
	}
	return defaultYieldWait
}

func (m *ExecSessionManager) maxOutputChars(maxOutputTokens int) int {
	if maxOutputTokens <= 0 {
		return 0
	}
	return maxOutputTokens * charsPerRequestedToken
}

func (s *ExecSession) wait() {
	err := s.Cmd.Wait()
	now := time.Now().UTC()
	exitCode := processExitCode(err)
	s.mu.Lock()
	s.ExitedAt = &now
	s.ExitCode = &exitCode
	s.mu.Unlock()
	close(s.done)
}

func (s *ExecSession) waitFor(wait time.Duration) {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case <-s.done:
	case <-s.Output.changed():
		select {
		case <-s.done:
		case <-time.After(10 * time.Millisecond):
		}
	case <-timer.C:
	}
}

func (s *ExecSession) waitUntilDone(timeout time.Duration) bool {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-s.done:
		return true
	case <-timer.C:
		return false
	}
}

func (s *ExecSession) format(output outputChunk, includeSessionID bool) string {
	lines := []string{}
	if includeSessionID {
		lines = append(lines, "session_id: "+s.ID)
	}
	lines = append(lines, "status: "+s.status())
	if exitCode := s.exitCodePtr(); exitCode != nil {
		lines = append(lines, fmt.Sprintf("exit_code: %d", *exitCode))
	}
	if output.TotalBytes > 0 || output.TruncatedBytes > 0 {
		lines = append(lines, fmt.Sprintf("output_bytes: %d", output.TotalBytes))
		if output.TruncatedBytes > 0 {
			lines = append(lines, "output_truncated: true")
			lines = append(lines, fmt.Sprintf("omitted_bytes: %d", output.TruncatedBytes))
		}
		lines = append(lines, "output:")
		lines = append(lines, strings.TrimRight(output.Text, "\n"))
	} else {
		lines = append(lines, "output: (no output)")
	}
	return strings.Join(lines, "\n")
}

func (s *ExecSession) status() string {
	if s.exited() {
		return "exited"
	}
	return "running"
}

func (s *ExecSession) exitCode() int {
	if exitCode := s.exitCodePtr(); exitCode != nil {
		return *exitCode
	}
	return 0
}

func (s *ExecSession) exitCodePtr() *int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ExitCode
}

func (s *ExecSession) exited() bool {
	select {
	case <-s.done:
		return true
	default:
		return false
	}
}

func (s *ExecSession) kill() {
	if s.Cmd == nil || s.Cmd.Process == nil {
		return
	}
	pid := s.Cmd.Process.Pid
	if pid > 0 {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
	_ = s.Cmd.Process.Kill()
}

func processExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

type outputChunk struct {
	Text           string
	TotalBytes     int
	TruncatedBytes int
}

type cappedOutput struct {
	mu      sync.Mutex
	notify  chan struct{}
	max     int
	dropped int
	content []byte
}

func newCappedOutput(max int) *cappedOutput {
	return &cappedOutput{max: max, notify: make(chan struct{})}
}

func (b *cappedOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.content = append(b.content, p...)
	if len(b.content) > b.max {
		drop := len(b.content) - b.max
		b.dropped += drop
		b.content = append([]byte(nil), b.content[drop:]...)
	}
	close(b.notify)
	b.notify = make(chan struct{})
	return len(p), nil
}

func (b *cappedOutput) Drain(maxChars int) outputChunk {
	b.mu.Lock()
	defer b.mu.Unlock()
	text := string(b.content)
	truncated := b.dropped
	if maxChars > 0 {
		runes := []rune(text)
		if len(runes) > maxChars {
			truncated += len([]byte(string(runes[:len(runes)-maxChars])))
			text = string(runes[len(runes)-maxChars:])
		}
	}
	total := b.dropped + len(b.content)
	b.content = nil
	b.dropped = 0
	return outputChunk{Text: text, TotalBytes: total, TruncatedBytes: truncated}
}

func (b *cappedOutput) changed() <-chan struct{} {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.content) > 0 {
		ready := make(chan struct{})
		close(ready)
		return ready
	}
	return b.notify
}
