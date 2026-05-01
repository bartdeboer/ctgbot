package broker

import (
	"context"
	"errors"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/v2/component"
)

func (b *Broker) Run(ctx context.Context) error {
	if err := b.ensureReady(); err != nil {
		return err
	}
	if b.components == nil {
		return fmt.Errorf("missing broker components")
	}
	sources := b.components.EventSources()
	if len(sources) == 0 {
		return fmt.Errorf("missing event sources")
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(sources))
	for _, source := range sources {
		source := source
		go func() {
			b.logf("v2 event source starting type=%s", source.Type())
			err := source.RunEvents(runCtx, func(eventCtx context.Context, event component.InboundEvent) error {
				_, handleErr := b.HandleEvent(eventCtx, event)
				if handleErr != nil {
					b.logf("v2 event failed source=%s provider_chat=%q provider_thread=%q external=%q err=%v", event.SourceType, event.ProviderChatID, event.ProviderThreadID, event.ExternalID, handleErr)
					if b.EventErrorHandler != nil {
						b.EventErrorHandler(eventCtx, event, handleErr)
					}
				}
				return handleErr
			})
			if err != nil && !errors.Is(err, context.Canceled) {
				errCh <- fmt.Errorf("event source %s: %w", source.Type(), err)
				return
			}
			b.logf("v2 event source stopped type=%s", source.Type())
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
