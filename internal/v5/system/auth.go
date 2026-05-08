package system

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
)

func (s *System) AuthComponent(ctx context.Context, ref string, runtimeKind string, homePath string, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	registration, loaded, err := s.ensureLoadedComponent(ctx, ref, runtimeKind, homePath)
	if err != nil {
		return err
	}
	auth, ok := loaded.Component.(component.Authenticator)
	if !ok {
		return fmt.Errorf("component does not support auth: %s", registration.Ref())
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return auth.Auth(ctx, callbackPort, callbackTimeout, stdout, stderr)
}

func (s *System) CheckComponentAuth(ctx context.Context, ref string, runtimeKind string, homePath string, stdout io.Writer, stderr io.Writer) error {
	registration, loaded, err := s.ensureLoadedComponent(ctx, ref, runtimeKind, homePath)
	if err != nil {
		return err
	}
	reporter, ok := loaded.Component.(component.AuthStatusReporter)
	if !ok {
		return fmt.Errorf("component does not support auth status: %s", registration.Ref())
	}
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return reporter.AuthStatus(ctx, stdout, stderr)
}

func (s *System) ensureLoadedComponent(ctx context.Context, ref string, runtimeKind string, homePath string) (*coremodel.Component, *component.Loaded, error) {
	registration, err := s.EnsureComponent(ctx, ref, runtimeKind, homePath)
	if err != nil {
		return nil, nil, err
	}
	loaded, err := s.ResolveComponent(ctx, registration.ID)
	if err != nil {
		return nil, nil, err
	}
	return registration, loaded, nil
}
