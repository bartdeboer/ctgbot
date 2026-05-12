package agentcommon

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

func Thread(ctx context.Context, storage repository.Storage, req commandengine.Request, missingPrefix string) (*coremodel.Thread, error) {
	if storage == nil {
		return nil, fmt.Errorf("missing %s storage", cleanPrefix(missingPrefix))
	}
	threadID := req.Context.ThreadID
	if threadID.IsNull() {
		threadID = req.Context.SandboxID
	}
	if threadID.IsNull() {
		return nil, fmt.Errorf("missing thread id")
	}
	thread, err := storage.Threads().GetByID(ctx, threadID)
	if err != nil {
		return nil, err
	}
	if thread == nil {
		return nil, fmt.Errorf("thread not found: %s", threadID)
	}
	return thread, nil
}

func ThreadWorkspace(ctx context.Context, storage repository.Storage, resolveWorkspace func(context.Context, coremodel.Chat) (string, error), req commandengine.Request, missingPrefix string) (*coremodel.Thread, string, error) {
	if resolveWorkspace == nil {
		return nil, "", fmt.Errorf("missing workspace resolver")
	}
	thread, err := Thread(ctx, storage, req, missingPrefix)
	if err != nil {
		return nil, "", err
	}
	chat, err := storage.Chats().GetByID(ctx, thread.ChatID)
	if err != nil {
		return nil, "", err
	}
	if chat == nil {
		return nil, "", fmt.Errorf("chat not found: %s", thread.ChatID)
	}
	workspacePath, err := resolveWorkspace(ctx, *chat)
	if err != nil {
		return nil, "", err
	}
	return thread, workspacePath, nil
}

func ProviderThreadID(componentID modeluuid.UUID, turnRuntime component.TurnRuntime) (string, error) {
	if turnRuntime == nil {
		return "", fmt.Errorf("missing turn runtime")
	}
	componentThreadID, ok, err := turnRuntime.ComponentThreadID(componentID)
	if err != nil || !ok {
		return "", err
	}
	return strings.TrimSpace(componentThreadID), nil
}

func BindProviderThreadID(componentID modeluuid.UUID, turnRuntime component.TurnRuntime, providerThreadID string) error {
	providerThreadID = strings.TrimSpace(providerThreadID)
	if providerThreadID == "" {
		return nil
	}
	if turnRuntime == nil {
		return fmt.Errorf("missing turn runtime")
	}
	return turnRuntime.BindComponentThreadID(componentID, providerThreadID)
}

func RuntimeNotices(ctx context.Context, runtime runtimepkg.Runtime, workspacePath string, threadID modeluuid.UUID, logf func(string, ...any)) []string {
	if runtime == nil {
		return nil
	}
	status, err := runtime.Status(ctx, workspacePath, threadID)
	if err != nil {
		if logf != nil {
			logf("runtime notice status check failed thread=%s err=%v", threadID, err)
		}
		return nil
	}
	return append([]string(nil), status.RuntimeNotices...)
}

func StopAfterTurn(runtime runtimepkg.Runtime, workspacePath string, threadID modeluuid.UUID, timeout time.Duration, logf func(string, ...any)) {
	if runtime == nil {
		return
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := runtime.Stop(stopCtx, workspacePath, threadID); err != nil && logf != nil {
		logf("stop-after-turn failed thread=%s err=%v", threadID, err)
	}
}

type JSONStateStore[S any] struct {
	Storage     repository.Storage
	ComponentID modeluuid.UUID
	Label       string
	Clean       func(S) S
	IsZero      func(S) bool
}

func (s JSONStateStore[S]) Load(ctx context.Context, threadID modeluuid.UUID) (*coremodel.ThreadComponentState, S, error) {
	var zero S
	if s.Storage == nil {
		return nil, zero, fmt.Errorf("missing %s storage", cleanPrefix(s.Label))
	}
	row, err := s.Storage.ThreadComponentStates().GetByThreadAndComponent(ctx, threadID, s.ComponentID)
	if err != nil {
		return nil, zero, err
	}
	if row == nil || strings.TrimSpace(row.StateJSON) == "" {
		return row, s.clean(zero), nil
	}
	var state S
	if err := json.Unmarshal([]byte(row.StateJSON), &state); err != nil {
		return nil, zero, fmt.Errorf("decode %s thread state thread=%s component=%s: %w", cleanPrefix(s.Label), threadID, s.ComponentID, err)
	}
	return row, s.clean(state), nil
}

func (s JSONStateStore[S]) Save(ctx context.Context, storage repository.Storage, threadID modeluuid.UUID, row *coremodel.ThreadComponentState, state S) error {
	if storage == nil {
		return fmt.Errorf("missing storage")
	}
	if threadID.IsNull() {
		return fmt.Errorf("missing thread id")
	}
	state = s.clean(state)
	if s.isZero(state) {
		return storage.ThreadComponentStates().DeleteByThreadAndComponent(ctx, threadID, s.ComponentID)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("encode %s thread state: %w", cleanPrefix(s.Label), err)
	}
	if row == nil {
		row = &coremodel.ThreadComponentState{ThreadID: threadID, ComponentID: s.ComponentID}
	}
	row.ThreadID = threadID
	row.ComponentID = s.ComponentID
	row.StateJSON = string(data)
	return storage.ThreadComponentStates().Save(ctx, row)
}

func (s JSONStateStore[S]) Update(ctx context.Context, threadID modeluuid.UUID, mutate func(*S)) error {
	if s.Storage == nil {
		return fmt.Errorf("missing %s storage", cleanPrefix(s.Label))
	}
	row, state, err := s.Load(ctx, threadID)
	if err != nil {
		return err
	}
	if mutate != nil {
		mutate(&state)
	}
	return s.Storage.Transaction(ctx, func(tx repository.Storage) error {
		return s.Save(ctx, tx, threadID, row, state)
	})
}

func (s JSONStateStore[S]) clean(state S) S {
	if s.Clean == nil {
		return state
	}
	return s.Clean(state)
}

func (s JSONStateStore[S]) isZero(state S) bool {
	if s.IsZero == nil {
		return false
	}
	return s.IsZero(state)
}

func cleanPrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "agent"
	}
	return prefix
}
