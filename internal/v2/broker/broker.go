// Package broker sketches the v2 routing layer.
package broker

import (
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/v2/component"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v2/repository"
)

type Broker struct {
	storage    repository.Storage
	components *component.Registry
	logfFunc   func(format string, args ...any)
}

type EventOutcome struct {
	Inbound  *coremodel.ThreadMessage
	Outbound []coremodel.ThreadMessage
	Blocked  bool
	Command  bool
}

func New(storage repository.Storage, components *component.Registry, logf func(format string, args ...any)) *Broker {
	return &Broker{
		storage:    storage,
		components: components,
		logfFunc:   logf,
	}
}

func (b *Broker) Components() *component.Registry {
	if b == nil {
		return nil
	}
	return b.components
}

func (b *Broker) ensureReady() error {
	if b == nil || b.storage == nil {
		return fmt.Errorf("missing broker storage")
	}
	return nil
}

func (b *Broker) logf(format string, args ...any) {
	if b != nil && b.logfFunc != nil {
		b.logfFunc(format, args...)
	}
}
