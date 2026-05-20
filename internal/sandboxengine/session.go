package sandboxengine

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

type SessionOptions struct {
	IdleTimeout time.Duration
	StopTimeout time.Duration
}

type Session struct {
	manager *SandboxManager
	sandbox *Sandbox
	name    string
	options SessionOptions
	once    sync.Once
}

type sandboxSessions struct {
	mu     sync.Mutex
	states map[string]*sandboxSessionState
}

type sandboxSessionState struct {
	active int
	timer  *time.Timer
}

func (m *SandboxManager) BeginSession(ctx context.Context, spec SandboxSpec, options SessionOptions) (*Session, error) {
	if m == nil {
		return nil, fmt.Errorf("missing sandbox manager")
	}
	if strings.TrimSpace(spec.Name) == "" {
		return nil, fmt.Errorf("missing sandbox name")
	}
	sandbox := m.CreateSandbox(&spec)
	sessions := m.ensureSessions()
	sessions.begin(sandbox.Name)
	session := &Session{
		manager: m,
		sandbox: sandbox,
		name:    sandbox.Name,
		options: options,
	}
	if _, err := sandbox.Ensure(ctx); err != nil {
		sessions.close(sandbox.Name, SessionOptions{}, nil)
		return nil, err
	}
	return session, nil
}

func (s *Session) Sandbox() *Sandbox {
	if s == nil {
		return nil
	}
	return s.sandbox
}

func (s *Session) Exec(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	if s == nil || s.sandbox == nil {
		return nil
	}
	return s.sandbox.Exec(ctx, stdout, stderr, name, args...)
}

func (s *Session) ExecTTY(ctx context.Context, stdout io.Writer, stderr io.Writer, name string, args ...string) error {
	if s == nil || s.sandbox == nil {
		return nil
	}
	return s.sandbox.ExecTTY(ctx, stdout, stderr, name, args...)
}

func (s *Session) CombinedOutput(ctx context.Context, name string, args ...string) ([]byte, error) {
	if s == nil || s.sandbox == nil {
		return nil, nil
	}
	return s.sandbox.CombinedOutput(ctx, name, args...)
}

func (s *Session) Close() error {
	if s == nil || s.manager == nil || s.name == "" {
		return nil
	}
	s.once.Do(func() {
		s.manager.ensureSessions().close(s.name, s.options, func(ctx context.Context) error {
			if s.sandbox == nil {
				return nil
			}
			return s.sandbox.Stop(ctx)
		})
	})
	return nil
}

func (m *SandboxManager) ensureSessions() *sandboxSessions {
	if m == nil {
		return &sandboxSessions{states: map[string]*sandboxSessionState{}}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sessions == nil {
		m.sessions = &sandboxSessions{states: map[string]*sandboxSessionState{}}
	}
	return m.sessions
}

func (s *sandboxSessions) begin(name string) {
	if s == nil || name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.states == nil {
		s.states = map[string]*sandboxSessionState{}
	}
	state := s.states[name]
	if state == nil {
		state = &sandboxSessionState{}
		s.states[name] = state
	}
	state.active++
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
}

func (s *sandboxSessions) close(name string, options SessionOptions, stop func(context.Context) error) {
	if s == nil || name == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[name]
	if state == nil {
		return
	}
	if state.active > 0 {
		state.active--
	}
	if state.active > 0 {
		return
	}
	if state.timer != nil {
		state.timer.Stop()
		state.timer = nil
	}
	if options.IdleTimeout <= 0 || stop == nil {
		delete(s.states, name)
		return
	}
	timeout := options.StopTimeout
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	state.timer = time.AfterFunc(options.IdleTimeout, func() {
		// A new session cancels a pending timer, but cannot cancel a stop callback
		// that has already started. That rare race can overlap Stop with a fresh
		// BeginSession/Ensure; callers should keep idle timeouts comfortably above
		// normal request cadence until sandbox stop/start becomes serialized here.
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()
		_ = stop(ctx)
		s.mu.Lock()
		current := s.states[name]
		if current == state && current.active == 0 {
			delete(s.states, name)
		}
		s.mu.Unlock()
	})
}
