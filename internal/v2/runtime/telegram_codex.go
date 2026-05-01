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
	v2runtimecomponent "github.com/bartdeboer/ctgbot/internal/v2/component/runtime"
	v2telegram "github.com/bartdeboer/ctgbot/internal/v2/component/telegram"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

type TelegramCodexOptions struct {
	Token                   string
	CodexProfile            string
	PollTimeout             time.Duration
	Actions                 v2runtimecomponent.Actions
	OperatorTelegramUserIDs []int64
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
	telegramComponent.RootUserIDs = opts.OperatorTelegramUserIDs
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

	runtimeComponent := v2runtimecomponent.New(opts.Actions)
	registry := v2component.NewRegistry(telegramComponent, codexComponent, runtimeComponent)
	defaultChatComponents := []coremodel.ChatComponent{
		{ComponentType: v2telegram.ComponentType, ProfileName: v2telegram.DefaultProfileName, Enabled: true},
		{ComponentType: v2codex.ComponentType, ProfileName: codexProfile, Enabled: true},
		{ComponentType: v2runtimecomponent.ComponentType, Enabled: true},
	}
	telegramComponent.EventErrorHandler = func(eventCtx context.Context, event v2component.InboundEvent, err error) {
		sendTelegramError(eventCtx, telegramComponent, event, err, logger)
	}
	broker := v2broker.New(rt.Storage, registry, defaultChatComponents, logger.Printf)

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("ctgbot v2 runtime starting")
	fmt.Printf("codex_profile: %s\n", profileHostPath)
	fmt.Printf("workspace_root: %s\n", workspaceRoot)
	fmt.Printf("image: %s\n", rt.Image)
	fmt.Println("telegram: configured")
	fmt.Printf("status: running telegram -> codex(%s) -> telegram\n", codexProfile)
	return broker.Run(runCtx)
}

func ensureRuntimeRows(ctx context.Context, rt *Runtime, codexProfile string) error {
	for _, componentType := range []string{v2telegram.ComponentType, v2codex.ComponentType, v2runtimecomponent.ComponentType} {
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
