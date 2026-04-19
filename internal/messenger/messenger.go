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
	Text              string
	Attachments       []IncomingAttachment
}

type OutboundMessage struct {
	Text string
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

type IncomingResult struct {
	Messages []OutboundMessage
}

type OutgoingMessage struct {
	SandboxID   modeluuid.UUID
	Text        string
	ContentType string
}

type OutgoingFile struct {
	SandboxID   modeluuid.UUID
	Filename    string
	Caption     string
	ContentType string
	Content     []byte
}

type ResolvedOutgoingMessage struct {
	ProviderChatID   string
	ProviderThreadID string
	Text             string
	ContentType      string
}

type ResolvedOutgoingFile struct {
	ProviderChatID   string
	ProviderThreadID string
	Filename         string
	Caption          string
	ContentType      string
	Content          []byte
}

type InboundChatProvider interface {
	ProviderType() string
	Run(ctx context.Context, onUpdate func(context.Context, IncomingUpdate) (IncomingResult, error)) error
}

type OutboundChatProvider interface {
	ProviderType() string
	SendText(ctx context.Context, msg ResolvedOutgoingMessage) error
	SendFile(ctx context.Context, file ResolvedOutgoingFile) error
	StartChatAction(ctx context.Context, target ChatTarget, action ChatAction) (func(), error)
}
