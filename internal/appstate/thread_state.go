package appstate

import (
	"context"
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/dbstorage"
)

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
