package inbound

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

// FilterInput is the already-admitted inbound message context passed to
// optional event filters. Channel/source resolution happens before this stage
// in broker admission; filters may drop or transform the event, but they do
// not discover routing state.
type FilterInput struct {
	Event         component.InboundEvent
	Chat          coremodel.Chat
	SourceBinding coremodel.ChatComponent
}

type FilterAction string

const (
	FilterActionPass       FilterAction = "pass"
	FilterActionDrop       FilterAction = "drop"
	FilterActionQuarantine FilterAction = "quarantine"
)

type FilterResult struct {
	Event   component.InboundEvent
	Action  FilterAction
	Reason  string
	Details []string
}

type Filter interface {
	FilterInbound(ctx context.Context, input FilterInput) (FilterResult, error)
}

func Pass(input FilterInput) FilterResult {
	return FilterResult{Event: input.Event, Action: FilterActionPass}
}

func Drop(input FilterInput, reason string, details ...string) FilterResult {
	return FilterResult{Event: input.Event, Action: FilterActionDrop, Reason: reason, Details: details}
}

func Quarantine(input FilterInput, reason string, details ...string) FilterResult {
	return FilterResult{Event: input.Event, Action: FilterActionQuarantine, Reason: reason, Details: details}
}
