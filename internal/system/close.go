package system

import (
	"context"
	"errors"

	backendruntime "github.com/bartdeboer/ctgbot/internal/runtime/backend"
)

// CloseNativeBackends ends host-owned inference processes, not Docker services.
func (s *System) CloseNativeBackends(ctx context.Context) error {
	var errs []error
	for _, factory := range s.Runtimes {
		if backend, ok := factory.(*backendruntime.Factory); ok {
			errs = append(errs, backend.Close(ctx))
		}
	}
	return errors.Join(errs...)
}

// EnableNativeBackends is called only by the resident run composition.
func (s *System) EnableNativeBackends() error {
	for _, factory := range s.Runtimes {
		if backend, ok := factory.(*backendruntime.Factory); ok {
			if err := backend.EnableNative(); err != nil {
				return err
			}
		}
	}
	return nil
}
