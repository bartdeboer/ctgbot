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
	v5gmail "github.com/bartdeboer/ctgbot/internal/v5/component/gmail"
	v5process "github.com/bartdeboer/ctgbot/internal/v5/component/process"
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
			stateRoot := fs.String("state-root", "", "ctgbot state root (default: <cwd>/.ctgbot)")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", v5codex.DefaultImage, "Codex runtime image")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			runCtx, stop := signal.NotifyContext(req.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			system, err := openV5SystemForRoutes(
				req,
				store,
				*stateRoot,
				*dbPath,
				resolveTelegramToken(*telegramToken, store),
				*codexImage,
				&runtimeProcessActions{
					stop:    stop,
					install: func(ctx context.Context) error { return runInstalledCtgbotCommand(ctx, "install") },
					upgrade: func(ctx context.Context) error { return runInstalledCtgbotCommand(ctx, "upgrade") },
				},
			)
			if err != nil {
				return err
			}
			if _, _, err := system.StartHostbridge(); err != nil {
				return fmt.Errorf("start v5 hostbridge: %w", err)
			}

			fmt.Println("ctgbot v5 runtime initialized")
			fmt.Printf("state_root: %s\n", system.StateRoot)
			fmt.Printf("database: %s\n", system.DBPath)

			logf := func(format string, args ...any) {}
			if system.Logger != nil {
				logf = system.Logger.Printf
			}
			return v5broker.New(system.Storage, system, logf).Run(runCtx)
		})

		b.Handle("v5 workspace set <workspace>", "Configure a v5 workspace", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 workspace set", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			path := fs.String("path", "", "Host workspace path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rootDir, err := filepath.Abs(".")
			if err != nil {
				return err
			}

			if strings.TrimSpace(*path) == "" {
				return fmt.Errorf("missing workspace path")
			}

			workspace, err := v5system.SaveWorkspace(rootDir, store, strings.TrimSpace(req.Params["workspace"]), strings.TrimSpace(*path))
			if err != nil {
				return err
			}
			fmt.Println("workspace saved")
			fmt.Printf("name: %s\n", workspace.Name)
			fmt.Printf("path: %s\n", workspace.Path)
			return nil
		})

		b.Handle("v5 workspace list", "List configured v5 workspaces", func(req *clir.Request) error {
			rootDir, err := filepath.Abs(".")
			if err != nil {
				return err
			}
			workspaces, err := v5system.LoadWorkspaces(rootDir, store)
			if err != nil {
				return err
			}
			configured := v5system.ConfiguredWorkspaces(store)
			names := make([]string, 0, len(workspaces))
			for name := range workspaces {
				names = append(names, name)
			}
			slices.Sort(names)
			if len(names) == 0 {
				fmt.Println("no workspaces")
				return nil
			}
			for _, name := range names {
				workspace := workspaces[name]
				_, ok := configured[name]
				fmt.Printf("%s\tpath=%s\tconfigured=%t\n", workspace.Name, workspace.Path, ok)
			}
			return nil
		})

		b.Handle("v5 component register <component>", "Register a v5 component instance", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 component register", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", v5codex.DefaultImage, "Codex runtime image")
			runtimeKind := fs.String("runtime", "", "Runtime kind for this registered component (docker or local)")
			homePath := fs.String("home", "", "Optional host component home override")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage, nil)
			if err != nil {
				return err
			}
			registration, err := system.EnsureComponent(req.Context(), strings.TrimSpace(req.Params["component"]), strings.TrimSpace(*runtimeKind), strings.TrimSpace(*homePath))
			if err != nil {
				return err
			}
			runtime, err := system.Runtime(registration.Runtime)
			if err != nil {
				return err
			}
			home := runtime.ComponentHome(*registration)

			fmt.Println("component registered")
			fmt.Printf("id: %s\n", registration.ID)
			fmt.Printf("ref: %s\n", registration.Ref())
			fmt.Printf("runtime: %s\n", registration.Runtime)
			fmt.Printf("home_path: %s\n", registration.HomePath)
			fmt.Printf("host_home: %s\n", home.Path)
			fmt.Printf("runtime_home: %s\n", runtime.RuntimeComponentHomePath(*registration, home))
			return nil
		})

		b.Handle("v5 component auth <component>", "Authenticate a v5 component instance", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 component auth", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			image := fs.String("image", v5codex.DefaultImage, "auth runtime image")
			callbackPort := fs.Int("callback-port", v5codex.DefaultCallbackPort, "auth callback relay port")
			callbackTimeout := fs.Duration("callback-timeout", 10*time.Minute, "auth callback relay timeout")
			runtimeKind := fs.String("runtime", "", "Runtime kind for this component registration (default: preserve existing)")
			homePath := fs.String("home", "", "Optional host component home override")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *image, nil)
			if err != nil {
				return err
			}
			if err := system.AuthComponent(req.Context(), strings.TrimSpace(req.Params["component"]), strings.TrimSpace(*runtimeKind), strings.TrimSpace(*homePath), *callbackPort, *callbackTimeout, os.Stdout, os.Stderr); err != nil {
				return err
			}
			fmt.Println("component auth completed")
			return nil
		})

		b.Handle("v5 component list", "List registered v5 components", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 component list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, "", v5codex.DefaultImage, nil)
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
				runtime, err := system.Runtime(registration.Runtime)
				if err != nil {
					return err
				}
				home := runtime.ComponentHome(registration)
				fmt.Printf("%s\t%s\truntime=%s\tdefault=%t\n",
					registration.ID,
					registration.Ref(),
					runtime.Kind(),
					registration.IsDefault,
				)
				fmt.Printf("\thost_home=%s\thome_path=%s\n", home.Path, registration.HomePath)
			}
			return nil
		})

		b.Handle("v5 chat create <label>", "Create a v5 chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 chat create", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, "", v5codex.DefaultImage, nil)
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
			fmt.Printf("workspace: %s\n", chat.Workspace)
			return nil
		})

		b.Handle("v5 chat list", "List v5 chats", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 chat list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, "", v5codex.DefaultImage, nil)
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
				fmt.Printf("%s\t%s\tworkspace=%s\tenabled=%t\n", chat.ID, chat.Label, chat.Workspace, chat.Enabled)
			}
			return nil
		})

		b.Handle("v5 chat <chatID> workspace set <workspace>", "Assign a named workspace to a v5 chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 chat workspace set", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, "", v5codex.DefaultImage, nil)
			if err != nil {
				return err
			}
			chatID, err := modeluuid.Parse(strings.TrimSpace(req.Params["chatID"]))
			if err != nil {
				return fmt.Errorf("parse chat id: %w", err)
			}
			chat, err := system.SetChatWorkspace(req.Context(), chatID, strings.TrimSpace(req.Params["workspace"]))
			if err != nil {
				return err
			}
			fmt.Println("chat workspace updated")
			fmt.Printf("chat_id: %s\n", chat.ID)
			fmt.Printf("workspace: %s\n", chat.Workspace)
			return nil
		})

		b.Handle("v5 chat <chatID> workspace clear", "Clear the named workspace from a v5 chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 chat workspace clear", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, "", v5codex.DefaultImage, nil)
			if err != nil {
				return err
			}
			chatID, err := modeluuid.Parse(strings.TrimSpace(req.Params["chatID"]))
			if err != nil {
				return fmt.Errorf("parse chat id: %w", err)
			}
			chat, err := system.SetChatWorkspace(req.Context(), chatID, "")
			if err != nil {
				return err
			}
			fmt.Println("chat workspace cleared")
			fmt.Printf("chat_id: %s\n", chat.ID)
			return nil
		})

		b.Handle("v5 chat <chatID> component add <role> <component>", "Bind a registered component to a chat by role", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v5 chat component add", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			externalChatID := fs.String("external-chat-id", "", "External provider chat id for source/relay bindings")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", v5codex.DefaultImage, "Codex runtime image")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage, nil)
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
				fmt.Printf("runtime: %s\n", registration.Runtime)
				fmt.Printf("home_path: %s\n", registration.HomePath)
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
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "v5 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openV5SystemForRoutes(req, store, *stateRoot, *dbPath, "", v5codex.DefaultImage, nil)
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
				runtimeKind := ""
				if registration != nil {
					ref = registration.Ref()
					runtimeKind = registration.Runtime
				}
				fmt.Printf("%s\truntime=%s\trole=%s\texternal_chat_id=%s\n", ref, runtimeKind, binding.Role, binding.ExternalChatID)
			}
			return nil
		})
	})
}

func openV5SystemForRoutes(req *clir.Request, store *clistate.Store, stateRoot string, dbPath string, telegramToken string, codexImage string, processActions v5process.Actions) (*v5system.System, error) {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	system, err := v5system.Open(req.Context(), stateRoot, dbPath, store, logger)
	if err != nil {
		return nil, err
	}
	system.Registry, err = newV5Registry(req.Context(), system, telegramToken, codexImage, processActions)
	if err != nil {
		return nil, err
	}
	return system, nil
}

func newV5Registry(ctx context.Context, system *v5system.System, telegramToken string, codexImage string, processActions v5process.Actions) (*component.Registry, error) {
	registry := component.NewRegistry()

	auxStorage := gormstorage.New(system.DB)
	if err := auxStorage.AutoMigrate(ctx); err != nil {
		return nil, err
	}

	if err := registry.Add(v5telegram.Type, func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
		return v5telegram.New(ctx, registration, runtime, home, storage, telegramToken, system.Config, auxStorage.TelegramUpdates(), system.Logger)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(v5codex.Type, func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
		return v5codex.New(ctx, registration, runtime, home, storage, system.Config, system.ResolveChatWorkspace, system.Logger, codexImage)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(v5gmail.Type, func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
		return v5gmail.New(ctx, registration, runtime, home, storage, nil)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(v5process.Type, func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _, _ = ctx, registration, runtime, home, storage
		return v5process.New(processActions), nil
	}); err != nil {
		return nil, err
	}
	return registry, nil
}
