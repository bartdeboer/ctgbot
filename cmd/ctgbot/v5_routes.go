package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/bartdeboer/ctgbot/internal/dbstorage/gormstorage"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	v5broker "github.com/bartdeboer/ctgbot/internal/v5/broker"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	v5codex "github.com/bartdeboer/ctgbot/internal/v5/component/codex"
	v5telegram "github.com/bartdeboer/ctgbot/internal/v5/component/telegram"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	v5system "github.com/bartdeboer/ctgbot/internal/v5/system"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerV5Routes(r *clir.Router, store *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("v5 run", "Run the v5 ctgbot runtime", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 run", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v5 state root (default: <cwd>/.ctgbot/v5)")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", v5codex.DefaultImage, "Codex runtime image")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage)
			if err != nil {
				return err
			}

			fmt.Println("ctgbot v5 runtime initialized")
			fmt.Printf("state_root: %s\n", system.StateRoot)
			fmt.Printf("database: %s\n", system.DBPath)

			runCtx, stop := signal.NotifyContext(req.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			logf := func(format string, args ...any) {}
			if system.Logger != nil {
				logf = system.Logger.Printf
			}
			return v5broker.New(system.Storage, system, logf).Run(runCtx)
		})

		b.Handle("v5 profile set <profile>", "Configure a v5 profile", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 profile set", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			runtimeKind := fs.String("runtime", "", "Runtime kind for this profile (docker or local)")
			homePath := fs.String("home-path", "", "Optional host profile root override")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rootDir, err := filepath.Abs(".")
			if err != nil {
				return err
			}

			runtimeSet := false
			homePathSet := false
			fs.Visit(func(f *flag.Flag) {
				switch f.Name {
				case "runtime":
					runtimeSet = true
				case "home-path":
					homePathSet = true
				}
			})
			if !runtimeSet && !homePathSet {
				return fmt.Errorf("provide --runtime and/or --home-path")
			}

			name := strings.TrimSpace(req.Params["profile"])
			configured := v5system.ConfiguredProfiles(store)
			settings := configured[name]
			if runtimeSet {
				value := strings.TrimSpace(*runtimeKind)
				if value == "" {
					return fmt.Errorf("missing runtime value")
				}
				settings.Runtime = value
			}
			if homePathSet {
				settings.HomePath = strings.TrimSpace(*homePath)
			}

			profile, err := v5system.SaveProfile(rootDir, store, name, settings.Runtime, settings.HomePath)
			if err != nil {
				return err
			}
			fmt.Println("profile saved")
			fmt.Printf("name: %s\n", profile.Name)
			fmt.Printf("runtime: %s\n", profile.Runtime)
			fmt.Printf("root: %s\n", profile.Root)
			fmt.Printf("home_path: %s\n", settings.HomePath)
			return nil
		})

		b.Handle("v5 profile list", "List configured v5 profiles", func(req *clir.Request) error {
			rootDir, err := filepath.Abs(".")
			if err != nil {
				return err
			}
			profiles, err := v5system.LoadProfiles(rootDir, store)
			if err != nil {
				return err
			}
			configured := v5system.ConfiguredProfiles(store)
			names := make([]string, 0, len(profiles))
			for name := range profiles {
				names = append(names, name)
			}
			slices.Sort(names)
			for _, name := range names {
				profile := profiles[name]
				settings, ok := configured[name]
				fmt.Printf("%s\truntime=%s\troot=%s\thome_path=%s\tconfigured=%t\n",
					profile.Name,
					profile.Runtime,
					profile.Root,
					settings.HomePath,
					ok,
				)
			}
			return nil
		})

		b.Handle("v5 component register <component>", "Register a v5 component instance", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 component register", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v5 state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", v5codex.DefaultImage, "Codex runtime image")
			profileName := fs.String("profile", "", "Profile for this registered component (default: preserve existing or default)")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage)
			if err != nil {
				return err
			}
			registration, err := system.EnsureComponent(req.Context(), strings.TrimSpace(req.Params["component"]), strings.TrimSpace(*profileName))
			if err != nil {
				return err
			}
			profile, err := system.Profile(registration.Profile)
			if err != nil {
				return err
			}
			runtime, err := system.Runtime(profile.Name)
			if err != nil {
				return err
			}
			home := runtime.ComponentHome(*registration)

			fmt.Println("component registered")
			fmt.Printf("id: %s\n", registration.ID)
			fmt.Printf("ref: %s\n", registration.Ref())
			fmt.Printf("profile: %s\n", registration.Profile)
			fmt.Printf("runtime: %s\n", runtime.Kind())
			fmt.Printf("host_home: %s\n", home.HostPath)
			fmt.Printf("container_home: %s\n", home.ContainerPath)
			return nil
		})

		b.Handle("v5 component auth <component>", "Authenticate a v5 component instance", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 component auth", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v5 state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			image := fs.String("image", v5codex.DefaultImage, "auth runtime image")
			callbackPort := fs.Int("callback-port", v5codex.DefaultCallbackPort, "auth callback relay port")
			callbackTimeout := fs.Duration("callback-timeout", 10*time.Minute, "auth callback relay timeout")
			profileName := fs.String("profile", "", "Profile for this component registration (default: preserve existing or default)")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *image)
			if err != nil {
				return err
			}
			if err := system.AuthComponent(req.Context(), strings.TrimSpace(req.Params["component"]), strings.TrimSpace(*profileName), *image, *callbackPort, *callbackTimeout, os.Stdout, os.Stderr); err != nil {
				return err
			}
			fmt.Println("component auth completed")
			return nil
		})

		b.Handle("v5 component list", "List registered v5 components", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 component list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v5 state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, "", v5codex.DefaultImage)
			if err != nil {
				return err
			}
			components, err := system.Storage.Components().ListEnabled(req.Context())
			if err != nil {
				return err
			}
			if len(components) == 0 {
				fmt.Println("no registered components")
				return nil
			}
			for _, registration := range components {
				profile, err := system.Profile(registration.Profile)
				if err != nil {
					return err
				}
				fmt.Printf("%s\t%s\tprofile=%s\truntime=%s\tdefault=%t\n",
					registration.ID,
					registration.Ref(),
					registration.Profile,
					profile.Runtime,
					registration.IsDefault,
				)
			}
			return nil
		})

		b.Handle("v5 chat create <label>", "Create a v5 chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 chat create", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v5 state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, "", v5codex.DefaultImage)
			if err != nil {
				return err
			}
			chat := &coremodel.Chat{
				Label:   strings.TrimSpace(req.Params["label"]),
				Enabled: true,
			}
			if chat.Label == "" {
				return fmt.Errorf("missing chat label")
			}
			if err := system.Storage.Chats().Save(req.Context(), chat); err != nil {
				return err
			}
			fmt.Println("chat created")
			fmt.Printf("id: %s\n", chat.ID)
			fmt.Printf("label: %s\n", chat.Label)
			return nil
		})

		b.Handle("v5 chat list", "List v5 chats", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 chat list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v5 state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, "", v5codex.DefaultImage)
			if err != nil {
				return err
			}
			chats, err := system.Storage.Chats().List(req.Context())
			if err != nil {
				return err
			}
			if len(chats) == 0 {
				fmt.Println("no chats")
				return nil
			}
			for _, chat := range chats {
				fmt.Printf("%s\t%s\tenabled=%t\n", chat.ID, chat.Label, chat.Enabled)
			}
			return nil
		})

		b.Handle("v5 chat <chatID> component add <role> <component>", "Bind a registered component to a chat by role", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 chat component add", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v5 state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			externalChatID := fs.String("external-chat-id", "", "External provider chat id for source/relay bindings")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", v5codex.DefaultImage, "Codex runtime image")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage)
			if err != nil {
				return err
			}
			chatID, err := modeluuid.Parse(strings.TrimSpace(req.Params["chatID"]))
			if err != nil {
				return fmt.Errorf("parse chat id: %w", err)
			}
			role := coremodel.ChatComponentRole(strings.TrimSpace(req.Params["role"]))
			binding, err := system.BindChatComponent(req.Context(), chatID, role, strings.TrimSpace(req.Params["component"]), strings.TrimSpace(*externalChatID))
			if err != nil {
				return err
			}
			registration, err := system.Storage.Components().GetByID(req.Context(), binding.ComponentID)
			if err != nil {
				return err
			}
			fmt.Println("chat component bound")
			fmt.Printf("chat_id: %s\n", binding.ChatID)
			if registration != nil {
				fmt.Printf("component: %s\n", registration.Ref())
				fmt.Printf("profile: %s\n", registration.Profile)
			} else {
				fmt.Printf("component_id: %s\n", binding.ComponentID)
			}
			fmt.Printf("role: %s\n", binding.Role)
			if binding.ExternalChatID != "" {
				fmt.Printf("external_chat_id: %s\n", binding.ExternalChatID)
			}
			return nil
		})

		b.Handle("v5 chat <chatID> component list", "List component bindings for a v5 chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 chat component list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v5 state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, "", v5codex.DefaultImage)
			if err != nil {
				return err
			}
			chatID, err := modeluuid.Parse(strings.TrimSpace(req.Params["chatID"]))
			if err != nil {
				return fmt.Errorf("parse chat id: %w", err)
			}
			bindings, err := system.Storage.ChatComponents().ListEnabledByChatID(req.Context(), chatID)
			if err != nil {
				return err
			}
			if len(bindings) == 0 {
				fmt.Println("no component bindings")
				return nil
			}
			for _, binding := range bindings {
				registration, err := system.Storage.Components().GetByID(req.Context(), binding.ComponentID)
				if err != nil {
					return err
				}
				ref := binding.ComponentID.String()
				profileName := ""
				if registration != nil {
					ref = registration.Ref()
					profileName = registration.Profile
				}
				fmt.Printf("%s\tprofile=%s\trole=%s\texternal_chat_id=%s\n", ref, profileName, binding.Role, binding.ExternalChatID)
			}
			return nil
		})
	})
}

func openV5SystemForRoutes(req *clir.Request, store *clistate.Store, stateRoot string, dbPath string, telegramToken string, codexImage string) (*v5system.System, error) {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	system, err := v5system.Open(req.Context(), stateRoot, dbPath, store, logger)
	if err != nil {
		return nil, err
	}
	system.Registry, err = newV5Registry(req.Context(), system, telegramToken, codexImage)
	if err != nil {
		return nil, err
	}
	return system, nil
}

func newV5Registry(ctx context.Context, system *v5system.System, telegramToken string, codexImage string) (*component.Registry, error) {
	registry := component.NewRegistry()

	auxStorage := gormstorage.New(system.DB)
	if err := auxStorage.AutoMigrate(ctx); err != nil {
		return nil, err
	}

	if err := registry.Add(v5telegram.Type, func(ctx context.Context, registration coremodel.Component, profile v5runtime.Profile, runtime v5runtime.Runtime, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
		return v5telegram.New(ctx, registration, profile, runtime, home, storage, telegramToken, system.Config, auxStorage.TelegramUpdates(), system.Logger)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(v5codex.Type, func(ctx context.Context, registration coremodel.Component, profile v5runtime.Profile, runtime v5runtime.Runtime, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
		return v5codex.New(ctx, registration, profile, runtime, home, storage, system.Config, system.Logger, codexImage)
	}); err != nil {
		return nil, err
	}
	return registry, nil
}
