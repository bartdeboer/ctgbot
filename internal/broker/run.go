package broker

import (
	"context"
	"errors"
	"fmt"

	component "github.com/bartdeboer/ctgbot/internal/component"
)

func (b *Broker) Run(ctx context.Context) error {
	if err := b.ensureReady(); err != nil {
		return err
	}
	sources, err := b.App.EnabledInboundSources(ctx)
	if err != nil {
		return err
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
			err := source.RunInbound(runCtx, func(eventCtx context.Context, event component.InboundEvent) error {
				_, handleErr := b.HandleInbound(eventCtx, event)
				if handleErr != nil {
					b.logf("inbound handling failed component=%s external_id=%q err=%v", event.ComponentID, event.ExternalID, handleErr)
				}
				return nil
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
