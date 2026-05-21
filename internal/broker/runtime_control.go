package broker

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

func (b *Broker) RefreshThreadRuntime(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	if err := b.ensureReady(); err != nil {
		return "", err
	}
	if threadID.IsNull() {
		return "", fmt.Errorf("missing thread id")
	}
	thread, err := b.App.Thread(ctx, threadID)
	if err != nil {
		return "", err
	}
	if thread == nil {
		return "", fmt.Errorf("thread not found: %s", threadID)
	}
	chat, err := b.App.Chat(ctx, thread.ChatID)
	if err != nil {
		return "", err
	}
	if chat == nil {
		return "", fmt.Errorf("chat not found: %s", thread.ChatID)
	}
	chatRuntime, err := b.runtimeForChat(ctx, *chat)
	if err != nil {
		return "", err
	}
	loadedByID := make(map[modeluuid.UUID]*component.Loaded, len(chatRuntime.Components))
	for _, loaded := range chatRuntime.Components {
		if loaded != nil {
			loadedByID[loaded.Registration.ID] = loaded
		}
	}
	var refreshed []string
	for _, agent := range chatRuntime.Agents {
		loaded := loadedByID[agent.ComponentID]
		if loaded == nil {
			continue
		}
		controller, ok := loaded.Component.(component.ThreadRuntimeController)
		if !ok {
			continue
		}
		if err := controller.RefreshThreadRuntime(ctx, component.ThreadRuntimeControlRequest{Chat: *chat, Thread: *thread, WorkspacePath: chatRuntime.Workspace}); err != nil {
			return "", err
		}
		refreshed = append(refreshed, loaded.Registration.Ref())
	}
	if len(refreshed) == 0 {
		return "", fmt.Errorf("no refreshable agent runtime for thread %s", threadID)
	}
	return "runtime refreshed\ncomponents: " + strings.Join(refreshed, ", "), nil
}
