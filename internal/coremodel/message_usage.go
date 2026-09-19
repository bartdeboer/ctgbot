package coremodel

import "github.com/bartdeboer/ctgbot/internal/modeluuid"

// MessageUsage is a provider-reported snapshot attached only at finalization.
// InputTokens includes cache reads/writes; counts are nil when unavailable.
// Scope distinguishes an invocation from a provider's accumulated snapshot.
type MessageUsage struct {
	SourceMessageID   modeluuid.UUID
	ProviderSessionID string
	Model             string
	Scope             string
	InputTokens       *int64
	CachedInputTokens *int64
	CacheWriteTokens  *int64
	OutputTokens      *int64
}
