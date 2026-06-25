package broker

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

var errNoThreadSandbox = errors.New("no thread sandbox")

type threadSandboxTarget struct {
	ref         string
	thread      coremodel.Thread
	sandbox     *sandboxengine.Sandbox
	keepRunning component.ThreadSandboxKeepRunning
	request     component.ThreadSandboxRequest
}

func (b *Broker) RefreshThreadRuntime(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	targets, err := b.threadSandboxTargets(ctx, threadID)
	if err != nil {
		return "", err
	}
	if err := sandboxengine.DeleteContainers(ctx, targetSandboxes(targets)); err != nil {
		return "", err
	}
	return "container refreshed\ncomponents: " + targetRefs(targets), nil
}

func (b *Broker) StartThreadRuntime(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	targets, err := b.threadSandboxTargets(ctx, threadID)
	if err != nil {
		return "", err
	}
	if err := sandboxengine.StartContainers(ctx, targetSandboxes(targets)); err != nil {
		return "", err
	}
	for _, target := range targets {
		if target.keepRunning == nil {
			continue
		}
		keep := true
		if err := target.keepRunning.SetThreadSandboxKeepRunning(ctx, target.request, &keep); err != nil {
			return "", err
		}
	}
	return "container started\nkeep_running: true\ncomponents: " + targetRefs(targets), nil
}

func (b *Broker) StopThreadRuntime(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	targets, err := b.threadSandboxTargets(ctx, threadID)
	if err != nil {
		return "", err
	}
	if err := sandboxengine.StopContainers(ctx, targetSandboxes(targets)); err != nil {
		return "", err
	}
	for _, target := range targets {
		if target.keepRunning == nil {
			continue
		}
		if err := target.keepRunning.SetThreadSandboxKeepRunning(ctx, target.request, nil); err != nil {
			return "", err
		}
	}
	return "container stopped\nkeep_running: false\ncomponents: " + targetRefs(targets), nil
}

func (b *Broker) RefreshAllThreadRuntimes(ctx context.Context) (string, error) {
	targets, skipped, err := b.allThreadSandboxTargets(ctx, false)
	if err != nil {
		return "", err
	}
	if err := sandboxengine.DeleteContainers(ctx, targetSandboxes(targets)); err != nil {
		return "", err
	}
	return sandboxAllText("containers refreshed", targets, skipped), nil
}

func (b *Broker) StartAllKeepRunningThreadRuntimes(ctx context.Context) (string, error) {
	targets, skipped, err := b.allThreadSandboxTargets(ctx, true)
	if err != nil {
		return "", err
	}
	if err := sandboxengine.StartContainers(ctx, targetSandboxes(targets)); err != nil {
		return "", err
	}
	return sandboxAllText("keep_running containers started", targets, skipped), nil
}

func (b *Broker) StopAllKeepRunningThreadRuntimes(ctx context.Context) (string, error) {
	targets, skipped, err := b.allThreadSandboxTargets(ctx, true)
	if err != nil {
		return "", err
	}
	if err := sandboxengine.StopContainers(ctx, targetSandboxes(targets)); err != nil {
		return "", err
	}
	return sandboxAllText("keep_running containers stopped", targets, skipped), nil
}

func (b *Broker) threadSandboxTargets(ctx context.Context, threadID modeluuid.UUID) ([]threadSandboxTarget, error) {
	thread, chat, runtime, err := b.threadRuntimeContext(ctx, threadID)
	if err != nil {
		return nil, err
	}
	targets, err := b.threadSandboxTargetsInRuntime(ctx, *chat, *thread, runtime)
	if err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		return nil, fmt.Errorf("%w: no controllable sandbox for thread %s", errNoThreadSandbox, threadID)
	}
	return targets, nil
}

func (b *Broker) threadRuntimeContext(ctx context.Context, threadID modeluuid.UUID) (*coremodel.Thread, *coremodel.Chat, *ChatRuntime, error) {
	if err := b.ensureReady(); err != nil {
		return nil, nil, nil, err
	}
	if threadID.IsNull() {
		return nil, nil, nil, fmt.Errorf("missing thread id")
	}
	thread, err := b.App.Thread(ctx, threadID)
	if err != nil {
		return nil, nil, nil, err
	}
	if thread == nil {
		return nil, nil, nil, fmt.Errorf("thread not found: %s", threadID)
	}
	chat, err := b.App.Chat(ctx, thread.ChatID)
	if err != nil {
		return nil, nil, nil, err
	}
	if chat == nil {
		return nil, nil, nil, fmt.Errorf("chat not found: %s", thread.ChatID)
	}
	runtime, err := b.runtimeForChat(ctx, *chat)
	if err != nil {
		return nil, nil, nil, err
	}
	return thread, chat, runtime, nil
}

func (b *Broker) allThreadSandboxTargets(ctx context.Context, onlyKeepRunning bool) ([]threadSandboxTarget, int, error) {
	if err := b.ensureReady(); err != nil {
		return nil, 0, err
	}
	chats, err := b.App.Chats(ctx)
	if err != nil {
		return nil, 0, err
	}
	var targets []threadSandboxTarget
	var skipped int
	for _, chat := range chats {
		if !chat.Enabled {
			skipped++
			continue
		}
		threads, err := b.App.Threads(ctx, chat.ID)
		if err != nil {
			return nil, 0, err
		}
		if len(threads) == 0 {
			continue
		}
		runtime, err := b.runtimeForChat(ctx, chat)
		if err != nil {
			return nil, 0, err
		}
		for _, thread := range threads {
			threadTargets, err := b.threadSandboxTargetsInRuntime(ctx, chat, thread, runtime)
			if err != nil {
				return nil, 0, err
			}
			if onlyKeepRunning {
				threadTargets, err = keepRunningSandboxTargets(ctx, threadTargets)
				if err != nil {
					return nil, 0, err
				}
			}
			if len(threadTargets) == 0 {
				skipped++
				continue
			}
			targets = append(targets, threadTargets...)
		}
	}
	return targets, skipped, nil
}

func (b *Broker) threadSandboxTargetsInRuntime(ctx context.Context, chat coremodel.Chat, thread coremodel.Thread, runtime *ChatRuntime) ([]threadSandboxTarget, error) {
	if runtime == nil {
		return nil, nil
	}
	loadedByID := make(map[modeluuid.UUID]*component.Loaded, len(runtime.Components))
	for _, loaded := range runtime.Components {
		if loaded != nil {
			loadedByID[loaded.Registration.ID] = loaded
		}
	}
	var targets []threadSandboxTarget
	for _, agent := range runtime.Agents {
		loaded := loadedByID[agent.ComponentID]
		if loaded == nil {
			continue
		}
		provider, ok := loaded.Component.(component.ThreadSandboxProvider)
		if !ok {
			continue
		}
		request := component.ThreadSandboxRequest{Chat: chat, Thread: thread, WorkspacePath: runtime.Workspace}
		sandbox, err := provider.ThreadSandbox(ctx, request)
		if err != nil {
			return nil, err
		}
		target := threadSandboxTarget{
			ref:     loaded.Registration.Ref(),
			thread:  thread,
			sandbox: sandbox,
			request: request,
		}
		if keepRunning, ok := loaded.Component.(component.ThreadSandboxKeepRunning); ok {
			target.keepRunning = keepRunning
		}
		targets = append(targets, target)
	}
	return targets, nil
}

func keepRunningSandboxTargets(ctx context.Context, targets []threadSandboxTarget) ([]threadSandboxTarget, error) {
	out := targets[:0]
	for _, target := range targets {
		if target.keepRunning == nil {
			continue
		}
		keep, err := target.keepRunning.ThreadSandboxKeepRunning(ctx, target.request)
		if err != nil {
			return nil, err
		}
		if keep {
			out = append(out, target)
		}
	}
	return out, nil
}

func targetSandboxes(targets []threadSandboxTarget) []*sandboxengine.Sandbox {
	sandboxes := make([]*sandboxengine.Sandbox, 0, len(targets))
	for _, target := range targets {
		if target.sandbox != nil {
			sandboxes = append(sandboxes, target.sandbox)
		}
	}
	return sandboxes
}

func targetRefs(targets []threadSandboxTarget) string {
	refs := make([]string, 0, len(targets))
	for _, target := range targets {
		refs = append(refs, target.ref)
	}
	return strings.Join(refs, ", ")
}

func sandboxAllText(header string, targets []threadSandboxTarget, skipped int) string {
	threads := map[modeluuid.UUID]struct{}{}
	for _, target := range targets {
		threads[target.thread.ID] = struct{}{}
	}
	return fmt.Sprintf("%s\nthreads: %d\ncontainers: %d\nskipped: %d", header, len(threads), len(targets), skipped)
}
