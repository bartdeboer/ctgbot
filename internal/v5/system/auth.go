package system

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/bartdeboer/ctgbot/internal/v5/component"
)

func (s *System) AuthComponent(ctx context.Context, ref string, profileName string, image string, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	_ = image
	registration, err := s.EnsureComponent(ctx, ref, profileName)
	if err != nil {
		return err
	}
	loaded, err := s.ResolveComponent(ctx, registration.ID)
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
