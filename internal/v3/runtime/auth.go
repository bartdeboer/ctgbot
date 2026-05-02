package runtime

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	v3component "github.com/bartdeboer/ctgbot/internal/v3/component"
)

type AuthOptions struct {
	Image           string
	CallbackPort    int
	CallbackTimeout time.Duration
	SandboxManager  sandboxengine.Manager
	Stdout          io.Writer
	Stderr          io.Writer
}

func (r *Runtime) AuthComponent(ctx context.Context, ref string, opts AuthOptions) error {
	if opts.SandboxManager == nil {
		return fmt.Errorf("missing sandbox manager")
	}
	componentRow, err := r.EnsureComponent(ctx, ref)
	if err != nil {
		return err
	}
	instance, err := r.ResolveComponent(ctx, componentRow.ID)
	if err != nil {
		return err
	}
	auth, ok := instance.Implementation.(v3component.Authenticator)
	if !ok {
		return fmt.Errorf("component does not support auth: %s", componentRow.Ref())
	}
	stdout := opts.Stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := opts.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	return auth.Auth(ctx, v3component.AuthRequest{
		Registration:    instance.Registration,
		Home:            instance.Home,
		Image:           opts.Image,
		CallbackPort:    opts.CallbackPort,
		CallbackTimeout: opts.CallbackTimeout,
		SandboxManager:  opts.SandboxManager,
		Stdout:          stdout,
		Stderr:          stderr,
	})
}
