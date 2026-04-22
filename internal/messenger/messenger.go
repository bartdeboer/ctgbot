package messenger

import (
	"context"
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

type InboundPayload struct {
	ProviderType      string
	ProviderChatID    string
	ProviderThreadID  string
	ProviderMessageID string
	ChatLabel         string
	UserLabel         string
	UserID            int64
	IsAdmin           bool
	Text              TextMessage
	Attachments       []Media
}

type OutboundPayload struct {
	ProviderChatID   string
	ProviderThreadID string
	Text             TextMessage
	Attachments      []Media
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
