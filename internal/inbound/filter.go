package inbound

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

// Envelope carries an inbound event through broker-owned filters.
//
// Filters may enrich it with routing data or, later, normalize/transform the
// event before the broker turns it into a thread message.
type Envelope struct {
	Event         component.InboundEvent
	SourceBinding *coremodel.ChatComponent
	Chat          *coremodel.Chat
}

type FilterResult struct {
	Envelope Envelope
	Drop     bool
	Reason   string
	Details  []string
}

type Filter interface {
	FilterInbound(ctx context.Context, envelope Envelope) (FilterResult, error)
}

func Pass(envelope Envelope) FilterResult {
	return FilterResult{Envelope: envelope}
}

func Drop(envelope Envelope, reason string, details ...string) FilterResult {
	return FilterResult{Envelope: envelope, Drop: true, Reason: reason, Details: details}
}
