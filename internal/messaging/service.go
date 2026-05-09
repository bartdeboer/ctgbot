package messaging

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

// Service is the core thread-oriented messaging contract.
//
// Different adapters should call this same interface:
//
// - hostbridge commands
// - remote HTTP clients
// - future web clients
// - possible agent-facing command surfaces
type Service interface {
	ListThreads(ctx context.Context, actor message.Actor, req ListThreadsRequest) ([]ThreadSummary, error)
	ListMessages(ctx context.Context, actor message.Actor, threadID modeluuid.UUID, req ListMessagesRequest) (MessagePage, error)
	SendMessage(ctx context.Context, actor message.Actor, threadID modeluuid.UUID, req SendMessageRequest) (*SendMessageResult, error)
}
