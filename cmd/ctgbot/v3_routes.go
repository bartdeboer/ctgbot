package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	v3component "github.com/bartdeboer/ctgbot/internal/v3/component"
	v3codex "github.com/bartdeboer/ctgbot/internal/v3/component/codex"
	v3telegram "github.com/bartdeboer/ctgbot/internal/v3/component/telegram"
	"github.com/bartdeboer/ctgbot/internal/v3/coremodel"
	v3runtime "github.com/bartdeboer/ctgbot/internal/v3/runtime"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerV3Routes(r *clir.Router, store *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("v3 run", "Run the experimental v3 ctgbot runtime", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v3 run", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v3 state root (default: <cwd>/.ctgbot/v3)")
			dbPath := fs.String("db-path", "", "v3 SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", v3codex.DefaultImage, "Codex runtime image")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			token := resolveTelegramToken(*telegramToken, store)
			if token == "" {
				fmt.Println("status: runtime not started")
				fmt.Println("hint: provide --telegram-token or TELEGRAM_BOT_TOKEN")
				return nil
			}

			logger := log.New(os.Stdout, "", log.LstdFlags)
			rt, err := v3runtime.Open(req.Context(), v3runtime.OpenOptions{
				StateRoot: *stateRoot,
				DBPath:    *dbPath,
				Store:     store,
				Logger:    logger,
			})
			if err != nil {
				return err
			}
			rt.Registry = newV3Registry(rt, token, *codexImage)

			fmt.Println("ctgbot v3 runtime initialized")
			fmt.Printf("state_root: %s\n", rt.StateRoot)
			fmt.Printf("database: %s\n", rt.DBPath)

			runCtx, stop := signal.NotifyContext(req.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			return rt.Broker(logger.Printf).Run(runCtx)
		})

		b.Handle("v3 component register <component>", "Register a v3 component instance", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v3 component register", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v3 state root")
			dbPath := fs.String("db-path", "", "v3 SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", v3codex.DefaultImage, "Codex runtime image")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rt, err := openV3RuntimeForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage)
			if err != nil {
				return err
			}
			componentRow, err := rt.EnsureComponent(req.Context(), strings.TrimSpace(req.Params["component"]))
			if err != nil {
				return err
			}
			home, err := rt.Homes.Home(*componentRow)
			if err != nil {
				return err
			}
			fmt.Println("component registered")
			fmt.Printf("id: %s\n", componentRow.ID)
			fmt.Printf("ref: %s\n", componentRow.Ref())
			fmt.Printf("host_home: %s\n", home.HostPath)
			fmt.Printf("container_home: %s\n", home.ContainerPath)
			return nil
		})

		b.Handle("v3 component auth <component>", "Authenticate a v3 component instance", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v3 component auth", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v3 state root")
			dbPath := fs.String("db-path", "", "v3 SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			image := fs.String("image", v3codex.DefaultImage, "auth sandbox image")
			callbackPort := fs.Int("callback-port", v3codex.DefaultCallbackPort, "auth callback relay port")
			callbackTimeout := fs.Duration("callback-timeout", 10*time.Minute, "auth callback relay timeout")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rt, err := openV3RuntimeForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *image)
			if err != nil {
				return err
			}
			if err := rt.AuthComponent(req.Context(), strings.TrimSpace(req.Params["component"]), v3runtime.AuthOptions{
				Image:           *image,
				CallbackPort:    *callbackPort,
				CallbackTimeout: *callbackTimeout,
				SandboxManager:  rt.Sandboxes,
				Stdout:          os.Stdout,
				Stderr:          os.Stderr,
			}); err != nil {
				return err
			}
			fmt.Println("component auth completed")
			return nil
		})

		b.Handle("v3 component list", "List registered v3 components", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v3 component list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v3 state root")
			dbPath := fs.String("db-path", "", "v3 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			rt, err := openV3RuntimeForRoutes(req, store, *stateRoot, *dbPath, "", v3codex.DefaultImage)
			if err != nil {
				return err
			}
			components, err := rt.Storage.Components().ListEnabled(req.Context())
			if err != nil {
				return err
			}
			if len(components) == 0 {
				fmt.Println("no registered components")
				return nil
			}
			for _, componentRow := range components {
				fmt.Printf("%s\t%s\tdefault=%t\n", componentRow.ID, componentRow.Ref(), componentRow.IsDefault)
			}
			return nil
		})

		b.Handle("v3 chat create <label>", "Create a v3 chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v3 chat create", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v3 state root")
			dbPath := fs.String("db-path", "", "v3 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			rt, err := openV3RuntimeForRoutes(req, store, *stateRoot, *dbPath, "", v3codex.DefaultImage)
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
			if err := rt.Storage.Chats().Save(req.Context(), chat); err != nil {
				return err
			}
			fmt.Println("chat created")
			fmt.Printf("id: %s\n", chat.ID)
			fmt.Printf("label: %s\n", chat.Label)
			return nil
		})

		b.Handle("v3 chat list", "List v3 chats", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v3 chat list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v3 state root")
			dbPath := fs.String("db-path", "", "v3 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			rt, err := openV3RuntimeForRoutes(req, store, *stateRoot, *dbPath, "", v3codex.DefaultImage)
			if err != nil {
				return err
			}
			chats, err := rt.Storage.Chats().List(req.Context())
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

		b.Handle("v3 chat <chatID> component add <role> <component>", "Bind a registered component to a chat by role", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v3 chat component add", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v3 state root")
			dbPath := fs.String("db-path", "", "v3 SQLite DB path")
			externalChatID := fs.String("external-chat-id", "", "External provider chat id for source/relay bindings")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", v3codex.DefaultImage, "Codex runtime image")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			rt, err := openV3RuntimeForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage)
			if err != nil {
				return err
			}
			chatID, err := modeluuid.Parse(strings.TrimSpace(req.Params["chatID"]))
			if err != nil {
				return fmt.Errorf("parse chat id: %w", err)
			}
			role := coremodel.ChatComponentRole(strings.TrimSpace(req.Params["role"]))
			binding, err := rt.BindChatComponent(req.Context(), chatID, role, strings.TrimSpace(req.Params["component"]), strings.TrimSpace(*externalChatID))
			if err != nil {
				return err
			}
			componentRow, err := rt.Storage.Components().GetByID(req.Context(), binding.ComponentID)
			if err != nil {
				return err
			}
			fmt.Println("chat component bound")
			fmt.Printf("chat_id: %s\n", binding.ChatID)
			if componentRow != nil {
				fmt.Printf("component: %s\n", componentRow.Ref())
			} else {
				fmt.Printf("component_id: %s\n", binding.ComponentID)
			}
			fmt.Printf("role: %s\n", binding.Role)
			if binding.ExternalChatID != "" {
				fmt.Printf("external_chat_id: %s\n", binding.ExternalChatID)
			}
			return nil
		})

		b.Handle("v3 chat <chatID> component list", "List component bindings for a v3 chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v3 chat component list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v3 state root")
			dbPath := fs.String("db-path", "", "v3 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			rt, err := openV3RuntimeForRoutes(req, store, *stateRoot, *dbPath, "", v3codex.DefaultImage)
			if err != nil {
				return err
			}
			chatID, err := modeluuid.Parse(strings.TrimSpace(req.Params["chatID"]))
			if err != nil {
				return fmt.Errorf("parse chat id: %w", err)
			}
			bindings, err := rt.Storage.ChatComponents().ListEnabledByChatID(req.Context(), chatID)
			if err != nil {
				return err
			}
			if len(bindings) == 0 {
				fmt.Println("no component bindings")
				return nil
			}
			for _, binding := range bindings {
				componentRow, err := rt.Storage.Components().GetByID(req.Context(), binding.ComponentID)
				if err != nil {
					return err
				}
				ref := binding.ComponentID.String()
				if componentRow != nil {
					ref = componentRow.Ref()
				}
				fmt.Printf("%s\trole=%s\texternal_chat_id=%s\n", ref, binding.Role, binding.ExternalChatID)
			}
			return nil
		})
	})
}

func openV3RuntimeForRoutes(req *clir.Request, store *clistate.Store, stateRoot string, dbPath string, telegramToken string, codexImage string) (*v3runtime.Runtime, error) {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	rt, err := v3runtime.Open(req.Context(), v3runtime.OpenOptions{
		StateRoot: stateRoot,
		DBPath:    dbPath,
		Store:     store,
		Logger:    logger,
	})
	if err != nil {
		return nil, err
	}
	rt.Registry = newV3Registry(rt, telegramToken, codexImage)
	return rt, nil
}

func newV3Registry(rt *v3runtime.Runtime, telegramToken string, codexImage string) *v3component.Registry {
	if rt == nil {
		return v3component.NewRegistry(
			v3telegram.NewFactory(telegramToken, nil, nil, nil),
			v3codex.NewFactory(nil, nil, nil, nil, codexImage),
		)
	}
	return v3component.NewRegistry(
		v3telegram.NewFactory(telegramToken, rt.Config, rt.TelegramUpdates, rt.Logger),
		v3codex.NewFactory(rt.Config, rt.Sandboxes, rt.Workspaces, rt.Logger, codexImage),
	)
}
