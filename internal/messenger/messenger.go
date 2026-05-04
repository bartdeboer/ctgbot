package messenger

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type TextMessage struct {
	Text string
}

type Media struct {
	Kind        string
	Filename    string
	ContentType string
	Content     []byte
	Syntax      string
}

type ChatAction string

const (
	ChatActionTyping         ChatAction = "typing"
	ChatActionUploadDocument ChatAction = "upload_document"
)

type ChatTarget struct {
	ProviderChatID   string
	ProviderThreadID string
}

type Actor struct {
	ID    string
	Label string
	Roles []simplerbac.Role
}

func (a Actor) HasRole(role simplerbac.Role) bool {
	if role == "" {
		return false
	}
	for _, candidate := range a.Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

type InboundPayload struct {
	ProviderType      string
	ProviderChatID    string
	ProviderThreadID  string
	ProviderMessageID string
	ChatLabel         string
	Actor             Actor
	UserLabel         string
	UserID            int64
	IsAdmin           bool
	Text              TextMessage
	Attachments       []Media
}

func (p InboundPayload) ResolvedActor() Actor {
	actor := p.Actor
	if strings.TrimSpace(actor.ID) == "" {
		if p.UserID != 0 {
			actor.ID = fmt.Sprintf("%d", p.UserID)
		} else {
			actor.ID = strings.TrimSpace(p.UserLabel)
		}
	}
	if strings.TrimSpace(actor.Label) == "" {
		actor.Label = strings.TrimSpace(p.UserLabel)
	}
	if len(actor.Roles) == 0 {
		actor.Roles = []simplerbac.Role{simplerbac.RoleUser}
		if p.IsAdmin {
			actor.Roles = append(actor.Roles, simplerbac.RoleRoot)
		}
	}
	return actor
}

type OutboundPayload struct {
	ProviderChatID   string
	ProviderThreadID string
	Text             TextMessage
	Attachments      []Media
}

func (p OutboundPayload) IsZero() bool {
	return strings.TrimSpace(p.Text.Text) == "" && len(p.Attachments) == 0
}

type InboundChatProvider interface {
	ProviderType() string
	Run(ctx context.Context, onUpdate func(context.Context, InboundPayload) (OutboundPayload, error)) error
}

type OutboundChatProvider interface {
	ProviderType() string
	Send(ctx context.Context, payload OutboundPayload) error
	StartChatAction(ctx context.Context, target ChatTarget, action ChatAction) (func(), error)
}
