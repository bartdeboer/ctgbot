package message

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
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

type Actor = coremodel.Actor

type InboundPayload struct {
	ProviderType      string
	ProviderChatID    string
	ProviderThreadID  string
	ProviderMessageID string
	ChatLabel         string
	Actor             Actor
	Text              TextMessage
	Attachments       []Media
}

func (p InboundPayload) ResolvedActor() Actor {
	actor := p.Actor.Resolved()
	if len(actor.Roles) == 0 {
		actor.Roles = []simplerbac.Role{simplerbac.RoleUser}
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
