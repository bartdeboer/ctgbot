package messenger

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type IncomingMessage struct {
	ProviderType      string
	ProviderChatID    string
	ProviderThreadID  string
	Message           string
	ChatLabel         string
	UserLabel         string
	UserID            int64
	IsAdmin           bool
	ProviderMessageID string
}

type IncomingAttachment struct {
	Kind     string
	Filename string
	Content  []byte
}

type IncomingUpdate struct {
	ProviderType      string
	ProviderChatID    string
	ProviderThreadID  string
	ProviderMessageID string
	ChatLabel         string
	UserLabel         string
	UserID            int64
	IsAdmin           bool
	Text              string
	Attachments       []IncomingAttachment
}

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

type OutgoingMessage struct {
	SandboxID   modeluuid.UUID
	Text        string
	ContentType string
}

type OutgoingMedia struct {
	SandboxID   modeluuid.UUID
	Filename    string
	Caption     string
	ContentType string
	Syntax      string
	Content     []byte
}

type ResolvedOutgoingMessage struct {
	ProviderChatID   string
	ProviderThreadID string
	Text             string
	ContentType      string
}

type ResolvedOutgoingMedia struct {
	ProviderChatID   string
	ProviderThreadID string
	Filename         string
	Caption          string
	ContentType      string
	Syntax           string
	Content          []byte
}

type InboundChatProvider interface {
	ProviderType() string
	Run(ctx context.Context, onUpdate func(context.Context, InboundPayload) (OutboundPayload, error)) error
}

type OutboundChatProvider interface {
	ProviderType() string
	SendAgentResponse(ctx context.Context, msg ResolvedOutgoingMessage) error
	SendMedia(ctx context.Context, media ResolvedOutgoingMedia) error
	StartChatAction(ctx context.Context, target ChatTarget, action ChatAction) (func(), error)
}
