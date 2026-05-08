package component_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type fakeCLIAdminComponent struct {
	authCalls       int
	authStatusCalls int
	callbackPort    int
	callbackTimeout time.Duration
}

type plainCLIComponent struct{}

func (f *fakeCLIAdminComponent) Type() string { return "fake" }
func (plainCLIComponent) Type() string        { return "plain" }

func (f *fakeCLIAdminComponent) Auth(ctx context.Context, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	_, _, _ = ctx, stdout, stderr
	f.authCalls++
	f.callbackPort = callbackPort
	f.callbackTimeout = callbackTimeout
	return nil
}

func (f *fakeCLIAdminComponent) AuthStatus(ctx context.Context, stdout io.Writer, stderr io.Writer) error {
	_, _, _ = ctx, stdout, stderr
	f.authStatusCalls++
	return nil
}

func TestCLIAdminSurfaceRoutesAuthCommands(t *testing.T) {
	componentValue := &fakeCLIAdminComponent{}
	surface := component.NewCLIAdminSurface(componentValue)
	if surface == nil {
		t.Fatal("NewCLIAdminSurface() = nil")
	}

	bound := []commandset.BoundSurface{{
		Surface:       surface,
		ComponentRef:  "fake/work",
		ComponentType: "fake",
	}}
	engine, err := commandset.NewBoundEngineForSource(commandengine.SourceCLI, bound)
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}
	base := commandengine.Request{Context: commandengine.Context{
		Source: commandengine.SourceCLI,
		Actor:  commandengine.Actor{ID: "cli", Roles: []simplerbac.Role{simplerbac.RoleRoot}},
	}}

	result, err := engine.Run(context.Background(), base, []string{"fake", "signin", "--callback-port", "7777", "--callback-timeout", "3m"})
	if err != nil {
		t.Fatalf("Run(signin) error = %v", err)
	}
	if got, want := result.Text, "component auth completed"; got != want {
		t.Fatalf("Run(signin) text = %q, want %q", got, want)
	}
	if got, want := componentValue.authCalls, 1; got != want {
		t.Fatalf("auth calls = %d, want %d", got, want)
	}
	if got, want := componentValue.callbackPort, 7777; got != want {
		t.Fatalf("callback port = %d, want %d", got, want)
	}
	if got, want := componentValue.callbackTimeout, 3*time.Minute; got != want {
		t.Fatalf("callback timeout = %s, want %s", got, want)
	}

	if _, err := engine.Run(context.Background(), base, []string{"fake", "auth", "status"}); err != nil {
		t.Fatalf("Run(auth status) error = %v", err)
	}
	if got, want := componentValue.authStatusCalls, 1; got != want {
		t.Fatalf("auth status calls = %d, want %d", got, want)
	}
}

func TestCLIAdminSurfaceIsNilWithoutAdminCapabilities(t *testing.T) {
	if got := component.NewCLIAdminSurface(plainCLIComponent{}); got != nil {
		t.Fatalf("NewCLIAdminSurface() = %#v, want nil", got)
	}
}
