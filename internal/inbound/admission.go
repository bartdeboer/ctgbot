package inbound

import (
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
)

type RejectionAction string

const (
	RejectionDrop       RejectionAction = "drop"
	RejectionQuarantine RejectionAction = "quarantine"
)

type Rejection struct {
	Action        RejectionAction
	Event         component.InboundEvent
	Chat          *coremodel.Chat
	SourceBinding *coremodel.ChatComponent
	Reason        string
	NoticeText    string
	Details       []string
}

type Admission struct {
	Channel  Channel
	Rejected *Rejection
	Filters  []Filterer
}

func Reject(event component.InboundEvent, chat *coremodel.Chat, sourceBinding *coremodel.ChatComponent, action RejectionAction, reason string, details ...string) *Rejection {
	return &Rejection{
		Action:        action,
		Event:         event,
		Chat:          chat,
		SourceBinding: sourceBinding,
		Reason:        strings.TrimSpace(reason),
		Details:       append([]string(nil), details...),
	}
}

func RejectionFromFilter(channel Channel, current component.InboundEvent, result FilterResult) *Rejection {
	rejectedEvent := result.Event
	if rejectedEvent.ComponentID.IsNull() {
		rejectedEvent = current
	}
	action := RejectionDrop
	if result.Action == FilterActionQuarantine {
		action = RejectionQuarantine
	}
	rejection := Reject(rejectedEvent, &channel.Chat, &channel.SourceBinding, action, result.Reason, result.Details...)
	rejection.NoticeText = strings.TrimSpace(result.NoticeText)
	return rejection
}
