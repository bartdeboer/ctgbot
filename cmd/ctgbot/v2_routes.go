package main

import (
	"context"
	"flag"
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
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	v2broker "github.com/bartdeboer/ctgbot/internal/v2/broker"
	v2component "github.com/bartdeboer/ctgbot/internal/v2/component"
	v2codex "github.com/bartdeboer/ctgbot/internal/v2/component/codex"
	v2gmail "github.com/bartdeboer/ctgbot/internal/v2/component/gmail"
	v2telegram "github.com/bartdeboer/ctgbot/internal/v2/component/telegram"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v2/profilemanager"
	"github.com/bartdeboer/ctgbot/internal/v2/repository"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	v2ConfigName = "configv2"
	v2DBName     = "ctgbotv2.db"
)

func registerV2Routes(r *clir.Router) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("run", "Run the experimental v2 ctgbot runtime", func(req *clir.Request) error {
			fs := flag.NewFlagSet("run", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			dbPath := fs.String("db-path", "", "v2 SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexProfile := fs.String("codex-profile", "", "Codex component profile name")
			codexImage := fs.String("codex-image", v2codex.DefaultImage, "Codex runtime image")
			pollTimeout := fs.Duration("telegram-poll-timeout", 30*time.Second, "Telegram long-poll timeout")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			runtime, err := openV2Runtime(req.Context(), v2RuntimeOptions{DBPath: *dbPath, Image: *codexImage})
			if err != nil {
				return err
			}

			fmt.Println("ctgbot v2 runtime initialized")
			fmt.Printf("config: %s\n", runtime.ConfigPath)
			fmt.Printf("database: %s\n", runtime.DBPath)
			token := resolveV2TelegramToken(*telegramToken, runtime.Config)
			profileName := strings.TrimSpace(*codexProfile)
			if token == "" || profileName == "" {
				fmt.Println("status: runtime not started")
				fmt.Println("hint: provide --telegram-token or TELEGRAM_BOT_TOKEN and --codex-profile")
				return nil
			}
			return runV2TelegramCodex(req.Context(), runtime, token, profileName, *pollTimeout)
		})

		b.Handle("component auth <component> <profile>", "Prepare a v2 component profile for authentication", func(req *clir.Request) error {
			fs := flag.NewFlagSet("component auth", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			dbPath := fs.String("db-path", "", "v2 SQLite DB path")
			image := fs.String("image", v2codex.DefaultImage, "auth sandbox image")
			prepareOnly := fs.Bool("prepare-only", false, "Only create profile metadata and directories")
			callbackPort := fs.Int("callback-port", v2codex.DefaultCallbackPort, "auth callback relay port")
			callbackTimeout := fs.Duration("callback-timeout", 10*time.Minute, "auth callback relay timeout")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			runtime, err := openV2Runtime(req.Context(), v2RuntimeOptions{DBPath: *dbPath, Image: *image})
			if err != nil {
				return err
			}

			componentType := strings.TrimSpace(req.Params["component"])
			profileName := strings.TrimSpace(req.Params["profile"])
			if componentType == "" {
				return fmt.Errorf("missing component")
			}
			if profileName == "" {
				return fmt.Errorf("missing profile")
			}

			hostPath, err := runtime.Profiles.Ensure(componentType, profileName)
			if err != nil {
				return err
			}

			if err := runtime.Storage.Components().Save(req.Context(), &coremodel.Component{
				Type:    componentType,
				Enabled: true,
			}); err != nil {
				return err
			}
			if err := runtime.Storage.ComponentProfiles().Save(req.Context(), &coremodel.ComponentProfile{
				ComponentType: componentType,
				ProfileName:   profileName,
				Enabled:       true,
			}); err != nil {
				return err
			}

			fmt.Println("component profile ready")
			fmt.Printf("component: %s\n", componentType)
			fmt.Printf("profile: %s\n", profileName)
			fmt.Printf("host_path: %s\n", hostPath)
			fmt.Printf("container_path: %s\n", runtime.Profiles.ContainerPath())

			if *prepareOnly {
				fmt.Println("auth: prepare only")
				return nil
			}

			candidate := v2ComponentForType(componentType)
			auth, ok := candidate.(v2component.Authenticator)
			if !ok {
				fmt.Println("auth: not implemented yet")
				return nil
			}

			return auth.Auth(req.Context(), v2component.AuthRequest{
				ComponentType:        componentType,
				ProfileName:          profileName,
				ProfileHostPath:      hostPath,
				ProfileContainerPath: runtime.Profiles.ContainerPath(),
				Image:                runtime.Image,
				CallbackPort:         *callbackPort,
				CallbackTimeout:      *callbackTimeout,
				SandboxManager:       runtime.Sandboxes,
				Stdout:               os.Stdout,
				Stderr:               os.Stderr,
			})
		})
	})
}

type v2Runtime struct {
	StateRoot string

	ConfigPath string
	DBPath     string
	Image      string
	Config     *clistate.Store
	Storage    repository.Storage
	Profiles   *profilemanager.Manager
	Sandboxes  sandboxengine.Manager
}

type v2RuntimeOptions struct {
	DBPath string
	Image  string
}

func openV2Runtime(ctx context.Context, opts v2RuntimeOptions) (*v2Runtime, error) {
	rootPath, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	stateRoot := filepath.Join(rootPath, ".ctgbot")
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		return nil, err
	}

	config, err := clistate.NewCwd("ctgbot", v2ConfigName)
	if err != nil {
		return nil, err
	}
	if err := config.PersistString("version", "v2"); err != nil {
		return nil, err
	}

	resolvedDBPath := strings.TrimSpace(opts.DBPath)
	if resolvedDBPath == "" {
		resolvedDBPath = filepath.Join(stateRoot, v2DBName)
	} else if !filepath.IsAbs(resolvedDBPath) {
		resolvedDBPath = filepath.Join(rootPath, resolvedDBPath)
	}
	storage, err := openV2Storage(ctx, resolvedDBPath)
	if err != nil {
		return nil, err
	}

	image := strings.TrimSpace(opts.Image)
	if image == "" {
		image = v2codex.DefaultImage
	}

	return &v2Runtime{
		StateRoot:  stateRoot,
		ConfigPath: filepath.Join(stateRoot, v2ConfigName+".json"),
		DBPath:     resolvedDBPath,
		Image:      image,
		Config:     config,
		Storage:    storage,
		Profiles:   profilemanager.New(rootPath),
		Sandboxes:  sandboxengine.NewSandboxManager(log.New(os.Stdout, "", log.LstdFlags)),
	}, nil
}

func openV2Storage(ctx context.Context, dbPath string) (repository.Storage, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("missing db path")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	storage := repository.NewGORM(db)
	if err := storage.AutoMigrate(ctx); err != nil {
		return nil, err
	}
	return storage, nil
}

func runV2TelegramCodex(ctx context.Context, runtime *v2Runtime, token string, codexProfile string, pollTimeout time.Duration) error {
	if runtime == nil {
		return fmt.Errorf("missing v2 runtime")
	}
	profileHostPath, err := runtime.Profiles.HostPath(v2codex.ComponentType, codexProfile)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(profileHostPath, "auth.json")); err != nil {
		return fmt.Errorf("codex profile %q is not ready: %w", codexProfile, err)
	}
	if err := ensureV2RuntimeRows(ctx, runtime, codexProfile); err != nil {
		return err
	}

	api, err := telegramengine.NewTelegramAPIV2(token)
	if err != nil {
		return err
	}
	telegramComponent := v2telegram.New(api)
	telegramComponent.PollTimeout = pollTimeout
	workspaceRoot := filepath.Join(runtime.StateRoot, "v2", "workspaces")

	codexComponent := v2codex.New(v2codex.Config{
		ProfileName:          codexProfile,
		ProfileHostPath:      profileHostPath,
		ProfileContainerPath: runtime.Profiles.ContainerPath(),
		WorkspaceRoot:        workspaceRoot,
		Image:                runtime.Image,
		SandboxManager:       runtime.Sandboxes,
		StateStore:           runtime.Storage.ThreadComponentStates(),
	})

	components := v2component.NewRegistry(telegramComponent, codexComponent)
	broker := v2broker.New(runtime.Storage, components)
	broker.DefaultChatComponents = []coremodel.ChatComponent{
		{ComponentType: v2telegram.ComponentType, ProfileName: v2telegram.DefaultProfileName, Enabled: true},
		{ComponentType: v2codex.ComponentType, ProfileName: codexProfile, Enabled: true},
	}
	logger := log.New(os.Stdout, "", log.LstdFlags)
	broker.Logf = logger.Printf

	runCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	fmt.Println("ctgbot v2 runtime starting")
	fmt.Printf("codex_profile: %s\n", profileHostPath)
	fmt.Printf("workspace_root: %s\n", workspaceRoot)
	fmt.Printf("image: %s\n", runtime.Image)
	fmt.Println("telegram: configured")
	fmt.Printf("status: running telegram -> codex(%s) -> telegram\n", codexProfile)
	return telegramComponent.RunEvents(runCtx, func(eventCtx context.Context, event v2component.InboundEvent) error {
		_, err := broker.HandleEvent(eventCtx, event)
		if err != nil {
			logger.Printf("v2 event failed source=%s provider_chat=%q provider_thread=%q external=%q err=%v", event.SourceType, event.ProviderChatID, event.ProviderThreadID, event.ExternalID, err)
			sendV2TelegramError(eventCtx, telegramComponent, event, err, logger)
		}
		return nil
	})
}

func sendV2TelegramError(ctx context.Context, telegramComponent *v2telegram.Component, event v2component.InboundEvent, eventErr error, logger *log.Logger) {
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

func ensureV2RuntimeRows(ctx context.Context, runtime *v2Runtime, codexProfile string) error {
	for _, componentType := range []string{v2telegram.ComponentType, v2codex.ComponentType} {
		if err := runtime.Storage.Components().Save(ctx, &coremodel.Component{
			Type:    componentType,
			Enabled: true,
		}); err != nil {
			return err
		}
	}
	if err := runtime.Storage.ComponentProfiles().Save(ctx, &coremodel.ComponentProfile{
		ComponentType: v2telegram.ComponentType,
		ProfileName:   v2telegram.DefaultProfileName,
		Enabled:       true,
	}); err != nil {
		return err
	}
	return runtime.Storage.ComponentProfiles().Save(ctx, &coremodel.ComponentProfile{
		ComponentType: v2codex.ComponentType,
		ProfileName:   strings.TrimSpace(codexProfile),
		Enabled:       true,
	})
}

func resolveV2TelegramToken(flagValue string, config *clistate.Store) string {
	if token := strings.TrimSpace(flagValue); token != "" {
		return token
	}
	if token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")); token != "" {
		return token
	}
	if config == nil {
		return ""
	}
	return strings.TrimSpace(config.GetString("telegram.token", ""))
}

func v2ComponentForType(componentType string) v2component.Component {
	switch strings.ToLower(strings.TrimSpace(componentType)) {
	case v2codex.ComponentType:
		return v2codex.New()
	case v2gmail.ComponentType:
		return v2gmail.New(nil)
	default:
		return nil
	}
}
