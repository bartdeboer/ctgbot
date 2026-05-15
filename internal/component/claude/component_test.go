package claude

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

func TestClaudeBootstrapIncludesRuntimeNotices(t *testing.T) {
	text := claudeBootstrap("/workspace", component.TurnInstructions{
		RuntimeNotices: []string{"[Runtime notice] image stale"},
	})
	if !strings.Contains(text, "[Runtime notice] image stale") {
		t.Fatalf("bootstrap text = %q, want runtime notice", text)
	}
}

func TestPrepareHomeWritesNonEmptyDefaultBootstrap(t *testing.T) {
	home := t.TempDir()
	if err := PrepareHome(HomeSpec{HostHome: home}); err != nil {
		t.Fatalf("PrepareHome() error = %v", err)
	}
	body, err := os.ReadFile(filepath.Join(home, "ctgbot-bootstrap.md"))
	if err != nil {
		t.Fatalf("read bootstrap: %v", err)
	}
	if strings.TrimSpace(string(body)) == "" {
		t.Fatalf("bootstrap is empty")
	}
}

func TestAuthRunsClaudeSetupTokenWithRelay(t *testing.T) {
	runtime := &authRuntime{home: runtimepkg.Home{Path: t.TempDir()}}
	c := &Component{runtime: runtime}
	if err := c.Auth(context.Background(), 1234, time.Minute, io.Discard, io.Discard); err != nil {
		t.Fatalf("Auth() error = %v", err)
	}
	if got, want := runtime.relayPort, 1234; got != want {
		t.Fatalf("relay port = %d, want %d", got, want)
	}
	if got, want := runtime.relayTimeout, time.Minute; got != want {
		t.Fatalf("relay timeout = %s, want %s", got, want)
	}
	if !runtime.relayClosed {
		t.Fatalf("relay was not closed")
	}
	if got, want := runtime.execName, "env"; got != want {
		t.Fatalf("exec name = %q, want %q", got, want)
	}
	if got, want := strings.Join(runtime.execArgs, " "), "BROWSER=echo claude setup-token"; got != want {
		t.Fatalf("exec args = %q, want %q", got, want)
	}
}

type authRuntime struct {
	home         runtimepkg.Home
	relayPort    int
	relayTimeout time.Duration
	relayClosed  bool
	execName     string
	execArgs     []string
}

func (r *authRuntime) Kind() string                                     { return "docker" }
func (r *authRuntime) ComponentHome() runtimepkg.Home                   { return r.home }
func (r *authRuntime) RuntimeComponentHomePath() string                 { return "/profile/components/claude/claude" }
func (r *authRuntime) RuntimeWorkspacePath(workspacePath string) string { return workspacePath }
func (r *authRuntime) Refresh(context.Context, string, modeluuid.UUID) error {
	return nil
}
func (r *authRuntime) Start(context.Context, string, modeluuid.UUID) (runtimepkg.Status, error) {
	return runtimepkg.Status{}, nil
}
func (r *authRuntime) Stop(context.Context, string, modeluuid.UUID) error { return nil }
func (r *authRuntime) Interrupt(context.Context, string, modeluuid.UUID) (bool, error) {
	return false, nil
}
func (r *authRuntime) Status(context.Context, string, modeluuid.UUID) (runtimepkg.Status, error) {
	return runtimepkg.Status{}, nil
}
func (r *authRuntime) Exec(_ context.Context, _ string, _ modeluuid.UUID, _ commandengine.CommandExecutor, _ io.Writer, _ io.Writer, name string, args ...string) error {
	r.execName = name
	r.execArgs = append([]string(nil), args...)
	return nil
}
func (r *authRuntime) CombinedOutput(context.Context, string, modeluuid.UUID, commandengine.CommandExecutor, string, ...string) ([]byte, error) {
	return nil, nil
}
func (r *authRuntime) OpenHTTPRelayPort(_ context.Context, _ string, _ modeluuid.UUID, _ commandengine.CommandExecutor, callbackPort int, callbackTimeout time.Duration) (func(context.Context) error, error) {
	r.relayPort = callbackPort
	r.relayTimeout = callbackTimeout
	return func(context.Context) error {
		r.relayClosed = true
		return nil
	}, nil
}
