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
	ShortID         string         `json:"short_id"`
	ChatID          modeluuid.UUID `json:"chat_id"`
	ChatLabel       string         `json:"chat_label"`
	ThreadLabel     string         `json:"thread_label"`
	LastMessageAt   time.Time      `json:"last_message_at"`
	LastMessageText string         `json:"last_message_text"`
}

type ThreadStatus struct {
	ID          modeluuid.UUID          `json:"id"`
	ShortID     string                  `json:"short_id"`
	Label       string                  `json:"label"`
	ChatID      modeluuid.UUID          `json:"chat_id"`
	ChatShortID string                  `json:"chat_short_id"`
	ChatLabel   string                  `json:"chat_label"`
	ChatEnabled bool                    `json:"chat_enabled"`
	Components  []ThreadStatusComponent `json:"components"`
}

type ThreadStatusComponent struct {
	Ref              string `json:"ref"`
	Role             string `json:"role"`
	ExternalChatID   string `json:"external_chat_id"`
	ExternalThreadID string `json:"external_thread_id"`
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
