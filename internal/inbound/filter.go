package inbound

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

// FilterInput is the already-admitted inbound message context passed to
// optional content filters. Channel/source resolution happens before this
// stage in broker admission; filters may drop or transform the event, but they
// do not discover routing state.
type FilterInput struct {
	Event         component.InboundEvent
	Chat          coremodel.Chat
	SourceBinding coremodel.ChatComponent
}

type FilterResult struct {
	Event   component.InboundEvent
	Drop    bool
	Reason  string
	Details []string
}

type Filter interface {
	FilterInbound(ctx context.Context, input FilterInput) (FilterResult, error)
}

func Pass(input FilterInput) FilterResult {
	return FilterResult{Event: input.Event}
}

func Drop(input FilterInput, reason string, details ...string) FilterResult {
	return FilterResult{Event: input.Event, Drop: true, Reason: reason, Details: details}
}
