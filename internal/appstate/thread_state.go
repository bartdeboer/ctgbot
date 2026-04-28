package appstate

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/dbstorage"
)

func (t ThreadConfig) CodexModel(ctx context.Context) (string, error) {
	threads, err := t.threadStorage()
	if err != nil {
		return "", err
	}
	return threads.CodexModel(ctx, t.threadID)
}

func (t ThreadConfig) SetCodexModel(ctx context.Context, model string) error {
	threads, err := t.threadStorage()
	if err != nil {
		return err
	}
	return threads.SetCodexModel(ctx, t.threadID, strings.TrimSpace(model))
}

func (t ThreadConfig) CodexReasoningEffort(ctx context.Context) (string, error) {
	threads, err := t.threadStorage()
	if err != nil {
		return "", err
	}
	return threads.CodexReasoningEffort(ctx, t.threadID)
}

func (t ThreadConfig) SetCodexReasoningEffort(ctx context.Context, effort string) error {
	threads, err := t.threadStorage()
	if err != nil {
		return err
	}
	return threads.SetCodexReasoningEffort(ctx, t.threadID, strings.TrimSpace(effort))
}

func (t ThreadConfig) KeepRunning(ctx context.Context) (bool, error) {
	threads, err := t.threadStorage()
	if err != nil {
		return false, err
	}
	return threads.KeepRunning(ctx, t.threadID)
}

func (t ThreadConfig) SetKeepRunning(ctx context.Context, value bool) error {
	threads, err := t.threadStorage()
	if err != nil {
		return err
	}
	return threads.SetKeepRunning(ctx, t.threadID, value)
}

func (t ThreadConfig) threadStorage() (dbstorage.ThreadStorage, error) {
	if t.cfg == nil || t.cfg.Storage() == nil || t.cfg.Storage().Threads() == nil {
		return nil, fmt.Errorf("missing storage")
	}
	if t.threadID.IsNull() {
		return nil, fmt.Errorf("missing thread id")
	}
	return t.cfg.Storage().Threads(), nil
}
