package broker

import (
	"context"
	"errors"
	"fmt"

	v3component "github.com/bartdeboer/ctgbot/internal/v3/component"
)

func (b *Broker) Run(ctx context.Context) error {
	if err := b.ensureReady(); err != nil {
		return err
	}
	components, err := b.Storage.Components().ListEnabled(ctx)
	if err != nil {
		return err
	}

	var sources []v3component.InboundSource
	for _, registration := range components {
		instance, err := b.Resolver.ResolveComponent(ctx, registration.ID)
		if err != nil {
			return err
		}
		source, ok := instance.Implementation.(v3component.InboundSource)
		if ok {
			sources = append(sources, source)
		}
	}
	if len(sources) == 0 {
		return fmt.Errorf("missing inbound sources")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(sources))
	for _, source := range sources {
		source := source
		go func() {
			err := source.RunInbound(runCtx, func(eventCtx context.Context, event v3component.InboundEvent) error {
				_, handleErr := b.HandleInbound(eventCtx, event)
				return handleErr
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("source %s: %w", source.Type(), err)
				return
			}
			errCh <- nil
		}()
	}

	for range sources {
		select {
		case <-ctx.Done():
			cancel()
			return nil
		case err := <-errCh:
			if err != nil {
				cancel()
				return err
			}
		}
	}
	return nil
}
