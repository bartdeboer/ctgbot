package inbound

import (
	"context"
	"sort"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

// Channel is an admitted inbound channel: broker has already verified that the
// provider channel is bound to an enabled ctgbot chat with a visible relay.
type Channel struct {
	Chat          coremodel.Chat
	SourceBinding coremodel.ChatComponent
}

// ChannelEvent is the only context event filters receive. Filters may inspect
// the admitted channel and may drop or transform the event, but they do not
// discover routing state.
type ChannelEvent struct {
	Channel Channel
	Event   component.InboundEvent
}

type FilterAction string

const (
	FilterActionPass       FilterAction = "pass"
	FilterActionDrop       FilterAction = "drop"
	FilterActionQuarantine FilterAction = "quarantine"
)

type FilterResult struct {
	Event      component.InboundEvent
	Action     FilterAction
	Reason     string
	NoticeText string
	Details    []string
}

type Precedence interface {
	InboundFilterPrecedence() int
}

type Filterer interface {
	Precedence
	FilterInbound(ctx context.Context, event ChannelEvent) (FilterResult, error)
}

type FilterChain struct {
	filters []Filterer
}

func NewFilterChain(ctx context.Context, filters []Filterer) (FilterChain, *FilterResult, error) {
	_ = ctx
	chainFilters := make([]Filterer, 0, len(filters))
	for _, filter := range filters {
		if filter != nil {
			chainFilters = append(chainFilters, filter)
		}
	}
	sort.SliceStable(chainFilters, func(i, j int) bool {
		return chainFilters[i].InboundFilterPrecedence() < chainFilters[j].InboundFilterPrecedence()
	})
	return FilterChain{filters: chainFilters}, nil, nil
}

func (c FilterChain) Run(ctx context.Context, event ChannelEvent) (FilterResult, error) {
	current := event
	for _, filter := range c.filters {
		result, err := filter.FilterInbound(ctx, current)
		if err != nil {
			return FilterResult{}, err
		}
		switch result.Action {
		case "", FilterActionPass:
			if !result.Event.ComponentID.IsNull() {
				current.Event = result.Event
			}
			continue
		case FilterActionDrop, FilterActionQuarantine:
			if result.Event.ComponentID.IsNull() {
				result.Event = current.Event
			}
			return result, nil
		default:
			return Quarantine(current, "inbound-filter-config-error", "error=unknown filter action "+string(result.Action)), nil
		}
	}
	return Pass(current), nil
}

func Pass(event ChannelEvent) FilterResult {
	return FilterResult{Event: event.Event, Action: FilterActionPass}
}

func Drop(event ChannelEvent, reason string, details ...string) FilterResult {
	return FilterResult{Event: event.Event, Action: FilterActionDrop, Reason: reason, Details: details}
}

func DropWithNotice(event ChannelEvent, reason string, noticeText string, details ...string) FilterResult {
	return FilterResult{Event: event.Event, Action: FilterActionDrop, Reason: reason, NoticeText: noticeText, Details: details}
}

func Quarantine(event ChannelEvent, reason string, details ...string) FilterResult {
	return FilterResult{Event: event.Event, Action: FilterActionQuarantine, Reason: reason, Details: details}
}
