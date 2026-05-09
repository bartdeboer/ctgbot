package messaging

import (
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type ActorKind string

const (
	ActorKindHuman  ActorKind = "human"
	ActorKindAgent  ActorKind = "agent"
	ActorKindClient ActorKind = "client"
)

type Actor struct {
	ID    string
	Label string
	Kind  ActorKind
	Roles []simplerbac.Role
}

func (a Actor) Resolved() Actor {
	if strings.TrimSpace(a.ID) == "" {
		a.ID = strings.TrimSpace(a.Label)
	}
	if strings.TrimSpace(a.Label) == "" {
		a.Label = strings.TrimSpace(a.ID)
	}
	if strings.TrimSpace(string(a.Kind)) == "" {
		a.Kind = ActorKindClient
	}
	return a
}

type ThreadSummary struct {
	ID              modeluuid.UUID `json:"id"`
	ChatID          modeluuid.UUID `json:"chat_id"`
	ChatLabel       string         `json:"chat_label"`
	ThreadLabel     string         `json:"thread_label"`
	LastMessageAt   time.Time      `json:"last_message_at"`
	LastMessageText string         `json:"last_message_text"`
}

type MessageRecord struct {
	ID          modeluuid.UUID             `json:"id"`
	ChatID      modeluuid.UUID             `json:"chat_id"`
	ThreadID    modeluuid.UUID             `json:"thread_id"`
	ComponentID modeluuid.UUID             `json:"component_id"`
	Direction   coremodel.MessageDirection `json:"direction"`
	Kind        coremodel.MessageKind      `json:"kind"`
	ActorID     string                     `json:"actor_id"`
	ActorLabel  string                     `json:"actor_label"`
	Text        string                     `json:"text"`
	CreatedAt   time.Time                  `json:"created_at"`
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
	Messages   []MessageRecord `json:"messages"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

type SendMessageRequest struct {
	Text string `json:"text"`
}

type SendMessageResult struct {
	Message MessageRecord `json:"message"`
}
