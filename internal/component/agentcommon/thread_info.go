package agentcommon

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

type ThreadInfo struct{}

func (c *Core) threadInfo(ctx context.Context, req commandengine.Request, provider string) (commandengine.Result, error) {
	thread, err := Thread(ctx, c.Storage, req, provider)
	if err != nil {
		return commandengine.Result{}, err
	}
	text, err := ThreadUsageInfo(ctx, c.Storage, thread.ID, c.Registration.ID)
	return commandengine.Result{Text: text}, err
}

// ThreadUsageInfo reads only persisted final-response usage for the current session.
func ThreadUsageInfo(ctx context.Context, storage repository.Storage, threadID, componentID modeluuid.UUID) (string, error) {
	row, err := storage.Messages().LatestFinal(ctx, threadID, componentID)
	if err != nil {
		return "", err
	}
	mapping, err := storage.ThreadComponentMappings().GetByThreadAndComponent(ctx, threadID, componentID)
	if err != nil {
		return "", err
	}
	if row == nil || mapping == nil || strings.TrimSpace(mapping.ComponentThreadID) == "" || row.Usage.ProviderSessionID != strings.TrimSpace(mapping.ComponentThreadID) {
		return "No final-response usage recorded for the current provider session.", nil
	}
	return FormatMessageUsage(*row), nil
}

func FormatMessageUsage(m coremodel.ThreadMessage) string {
	count := func(v *int64) string {
		if v == nil {
			return "unavailable"
		}
		return fmt.Sprint(*v)
	}
	u := m.Usage
	lines := []string{"Last finalized response: " + m.CreatedAt.UTC().Format(time.RFC3339),
		"Provider session: " + u.ProviderSessionID, "Model: " + FirstNonEmpty(u.Model, "unavailable"),
		"Scope: " + FirstNonEmpty(u.Scope, "usage unavailable"),
		"Input (including cache): " + count(u.InputTokens), "Cached input: " + count(u.CachedInputTokens),
		"Cache write: " + count(u.CacheWriteTokens), "Output: " + count(u.OutputTokens)}
	if u.InputTokens != nil && u.CachedInputTokens != nil && *u.InputTokens > 0 && *u.CachedInputTokens <= *u.InputTokens {
		lines = append(lines, fmt.Sprintf("Cache-read share: %.1f%%", 100*float64(*u.CachedInputTokens)/float64(*u.InputTokens)))
	}
	lines = append(lines, "Response-associated snapshot; failed attempts without a final response are not included.")
	return strings.Join(lines, "\n")
}
