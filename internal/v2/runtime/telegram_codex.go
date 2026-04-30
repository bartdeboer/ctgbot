package runtime

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bartdeboer/ctgbot/internal/messenger/telegramengine"
	v2broker "github.com/bartdeboer/ctgbot/internal/v2/broker"
	v2component "github.com/bartdeboer/ctgbot/internal/v2/component"
	v2codex "github.com/bartdeboer/ctgbot/internal/v2/component/codex"
	v2telegram "github.com/bartdeboer/ctgbot/internal/v2/component/telegram"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

type TelegramCodexOptions struct {
	Token        string
	CodexProfile string
	PollTimeout  time.Duration
}

func RunTelegramCodex(ctx context.Context, rt *Runtime, opts TelegramCodexOptions) error {
	if rt == nil {
		return fmt.Errorf("missing v2 runtime")
	}
	token := strings.TrimSpace(opts.Token)
	if token == "" {
		return fmt.Errorf("missing telegram token")
	}
	codexProfile := strings.TrimSpace(opts.CodexProfile)
	if codexProfile == "" {
		return fmt.Errorf("missing codex profile")
	}

	profileHostPath, err := rt.Profiles.HostPath(v2codex.ComponentType, codexProfile)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(profileHostPath, "auth.json")); err != nil {
		return fmt.Errorf("codex profile %q is not ready: %w", codexProfile, err)
	}
	if err := ensureRuntimeRows(ctx, rt, codexProfile); err != nil {
		return err
	}

	api, err := telegramengine.NewTelegramAPIV2(token)
	if err != nil {
		return err
	}
	logger := log.New(os.Stdout, "", log.LstdFlags)
	telegramComponent := v2telegram.New(api)
	telegramComponent.PollTimeout = opts.PollTimeout
	telegramComponent.Logf = logger.Printf
	workspaceRoot := filepath.Join(rt.StateRoot, "v2", "workspaces")

	codexComponent := v2codex.New(v2codex.Config{
		ProfileName:          codexProfile,
		ProfileHostPath:      profileHostPath,
		ProfileContainerPath: rt.Profiles.ContainerPath(),
		WorkspaceRoot:        workspaceRoot,
		Image:                rt.Image,
		SandboxManager:       rt.Sandboxes,
		StateStore:           rt.Storage.ThreadComponentStates(),
	})

	components := v2component.NewRegistry(telegramComponent, codexComponent)
	broker := v2broker.New(rt.Storage, components)
	broker.DefaultChatComponents = []coremodel.ChatComponent{
		{ComponentType: v2telegram.ComponentType, ProfileName: v2telegram.DefaultProfileName, Enabled: true},
		{ComponentType: v2codex.ComponentType, ProfileName: codexProfile, Enabled: true},
	}
	broker.Logf = logger.Printf

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("ctgbot v2 runtime starting")
	fmt.Printf("codex_profile: %s\n", profileHostPath)
	fmt.Printf("workspace_root: %s\n", workspaceRoot)
	fmt.Printf("image: %s\n", rt.Image)
	fmt.Println("telegram: configured")
	fmt.Printf("status: running telegram -> codex(%s) -> telegram\n", codexProfile)
	return telegramComponent.RunEvents(runCtx, func(eventCtx context.Context, event v2component.InboundEvent) error {
		_, err := broker.HandleEvent(eventCtx, event)
		if err != nil {
			logger.Printf("v2 event failed source=%s provider_chat=%q provider_thread=%q external=%q err=%v", event.SourceType, event.ProviderChatID, event.ProviderThreadID, event.ExternalID, err)
			sendTelegramError(eventCtx, telegramComponent, event, err, logger)
		}
		return nil
	})
}

func ensureRuntimeRows(ctx context.Context, rt *Runtime, codexProfile string) error {
	for _, componentType := range []string{v2telegram.ComponentType, v2codex.ComponentType} {
		if err := rt.Storage.Components().Save(ctx, &coremodel.Component{
			Type:    componentType,
			Enabled: true,
		}); err != nil {
			return err
		}
	}
	if err := rt.Storage.ComponentProfiles().Save(ctx, &coremodel.ComponentProfile{
		ComponentType: v2telegram.ComponentType,
		ProfileName:   v2telegram.DefaultProfileName,
		Enabled:       true,
	}); err != nil {
		return err
	}
	return rt.Storage.ComponentProfiles().Save(ctx, &coremodel.ComponentProfile{
		ComponentType: v2codex.ComponentType,
		ProfileName:   strings.TrimSpace(codexProfile),
		Enabled:       true,
	})
}

func sendTelegramError(ctx context.Context, telegramComponent *v2telegram.Component, event v2component.InboundEvent, eventErr error, logger *log.Logger) {
	if telegramComponent == nil || telegramComponent.API == nil || eventErr == nil {
		return
	}
	chatID, err := strconv.ParseInt(strings.TrimSpace(event.ProviderChatID), 10, 64)
	if err != nil {
		return
	}
	threadID := 0
	if rawThreadID := strings.TrimSpace(event.ProviderThreadID); rawThreadID != "" {
		threadID, _ = strconv.Atoi(rawThreadID)
	}
	text := "conversation error: " + strings.TrimSpace(eventErr.Error())
	if len(text) > 3500 {
		text = text[:3500] + "..."
	}
	if err := telegramComponent.API.SendMessage(ctx, chatID, threadID, 0, text, ""); err != nil && logger != nil {
		logger.Printf("v2 telegram error reply failed chat=%d thread=%d err=%v", chatID, threadID, err)
	}
}
