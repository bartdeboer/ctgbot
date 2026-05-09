package messaging

import (
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func ResolveActor(actor coremodel.Actor) coremodel.Actor {
	return actor.Resolved()
}

type ThreadSummary struct {
	ID              modeluuid.UUID `json:"id"`
	ChatID          modeluuid.UUID `json:"chat_id"`
	ChatLabel       string         `json:"chat_label"`
	ThreadLabel     string         `json:"thread_label"`
	LastMessageAt   time.Time      `json:"last_message_at"`
	LastMessageText string         `json:"last_message_text"`
}

type ListThreadsRequest struct {
	Limit int    `json:"limit"`
	Query string `json:"query"`
}

type ListMessagesRequest struct {
	Cursor string `json:"cursor"`
	Limit  int    `json:"limit"`
}

type MessagePage struct {
	Messages   []coremodel.ThreadMessage `json:"messages"`
	NextCursor string                    `json:"next_cursor,omitempty"`
}

type SendMessageRequest struct {
	Text string `json:"text"`
}

type SendMessageResult struct {
	Message coremodel.ThreadMessage `json:"message"`
}
