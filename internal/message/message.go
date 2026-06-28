package message

import (
	"context"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type TextMessage struct {
	Text string
	// ContentType is a MIME-style rendering hint for outbound text, such as
	// text/plain, text/markdown, or text/html. Relays may ignore unsupported
	// values or fall back when their target cannot render the requested format.
	// For example, Telegram HTML means Telegram's limited HTML parse-mode subset,
	// not arbitrary browser HTML.
	ContentType string
	// Syntax is a language/syntax hint for renderers that support fenced code
	// blocks. It is intentionally a hint: for example, Telegram uses it only for
	// text/markdown output and ignores it for text/plain and text/html.
	Syntax string
}

type Media struct {
	Kind        string
	Filename    string
	ContentType string
	// Syntax is a language/syntax hint for textual media previews.
	Syntax            string
	ContentID         string
	Disposition       string
	Content           []byte
	Width             int
	Height            int
	DurationSeconds   int
	SupportsStreaming bool
	Thumbnail         *MediaThumbnail
}

type MediaThumbnail struct {
	Filename    string
	ContentType string
	Content     []byte
}

type ChatAction string

const (
	ChatActionTyping         ChatAction = "typing"
	ChatActionUploadDocument ChatAction = "upload_document"
)

type ChatTarget struct {
	ProviderChannelID string
	ProviderThreadID  string
}

type Actor = coremodel.Actor

type InboundPayload struct {
	ProviderType      string
	ProviderChannelID string
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
	ProviderChannelID           string
	ProviderThreadID            string
	SupersedesProviderMessageID string
	Role                        coremodel.MessageRole
	Kind                        coremodel.MessageKind
	Text                        TextMessage
	Attachments                 []Media
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
