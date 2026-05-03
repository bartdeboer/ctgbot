package main

import (
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

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	v4component "github.com/bartdeboer/ctgbot/internal/v4/component"
	v4codex "github.com/bartdeboer/ctgbot/internal/v4/component/codex"
	v4telegram "github.com/bartdeboer/ctgbot/internal/v4/component/telegram"
	"github.com/bartdeboer/ctgbot/internal/v4/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v4/profiles"
	v4runtime "github.com/bartdeboer/ctgbot/internal/v4/runtime"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerV4Routes(r *clir.Router, store *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("v4 run", "Run the v4 ctgbot runtime", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v4 run", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v4 state root (default: <cwd>/.ctgbot/v4)")
			dbPath := fs.String("db-path", "", "v4 SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", v4codex.DefaultImage, "Codex runtime image")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rt, err := openV4RuntimeForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage)
			if err != nil {
				return err
			}

			fmt.Println("ctgbot v4 runtime initialized")
			fmt.Printf("state_root: %s\n", rt.StateRoot)
			fmt.Printf("database: %s\n", rt.DBPath)

			runCtx, stop := signal.NotifyContext(req.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			logf := func(format string, args ...any) {}
			if rt.Logger != nil {
				logf = rt.Logger.Printf
			}
			return rt.Broker(logf).Run(runCtx)
		})

		b.Handle("v4 profile set <profile>", "Configure a v4 profile", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v4 profile set", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			runtimeDriver := fs.String("runtime", "", "Runtime driver for this profile (docker or local)")
			homePath := fs.String("home-path", "", "Optional host profile root override")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			manager, err := newV4ProfileManager(store)
			if err != nil {
				return err
			}
			name := strings.TrimSpace(req.Params["profile"])
			settings := manager.Configured()[name]
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

			if runtimeSet {
				value := strings.TrimSpace(*runtimeDriver)
				if value == "" {
					return fmt.Errorf("missing runtime value")
				}
				settings.Runtime = value
			}
			if homePathSet {
				settings.HomePath = strings.TrimSpace(*homePath)
			}
			if !runtimeSet && !homePathSet {
				return fmt.Errorf("provide --runtime and/or --home-path")
			}
			if err := manager.Set(name, settings); err != nil {
				return err
			}
			profile, err := manager.Resolve(name)
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

		b.Handle("v4 profile list", "List configured v4 profiles", func(req *clir.Request) error {
			manager, err := newV4ProfileManager(store)
			if err != nil {
				return err
			}

			configured := manager.Configured()
			names := make([]string, 0, len(configured)+1)
			for name := range configured {
				names = append(names, name)
			}
			if !slices.Contains(names, "default") {
				names = append(names, "default")
			}
			slices.Sort(names)
			if len(names) == 0 {
				fmt.Println("no profiles")
				return nil
			}
			for _, name := range names {
				profile, err := manager.Resolve(name)
				if err != nil {
					return err
				}
				settings, configuredProfile := configured[name]
				fmt.Printf("%s\truntime=%s\troot=%s\thome_path=%s\tconfigured=%t\n",
					profile.Name,
					profile.Runtime,
					profile.Root,
					settings.HomePath,
					configuredProfile,
				)
			}
			return nil
		})

		b.Handle("v4 component register <component>", "Register a v4 component instance", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v4 component register", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v4 state root")
			dbPath := fs.String("db-path", "", "v4 SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", v4codex.DefaultImage, "Codex runtime image")
			profileName := fs.String("profile", "", "Profile for this registered component (default: preserve existing or default)")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rt, err := openV4RuntimeForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage)
			if err != nil {
				return err
			}
			componentRow, err := rt.EnsureComponent(req.Context(), strings.TrimSpace(req.Params["component"]), strings.TrimSpace(*profileName))
			if err != nil {
				return err
			}
			home, err := rt.Homes.Home(*componentRow)
			if err != nil {
				return err
			}
			profile, err := rt.Profiles.Resolve(componentRow.Profile)
			if err != nil {
				return err
			}

			fmt.Println("component registered")
			fmt.Printf("id: %s\n", componentRow.ID)
			fmt.Printf("ref: %s\n", componentRow.Ref())
			fmt.Printf("profile: %s\n", componentRow.Profile)
			fmt.Printf("runtime: %s\n", profile.Runtime)
			fmt.Printf("host_home: %s\n", home.HostPath)
			fmt.Printf("container_home: %s\n", home.ContainerPath)
			return nil
		})

		b.Handle("v4 component auth <component>", "Authenticate a v4 component instance", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v4 component auth", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v4 state root")
			dbPath := fs.String("db-path", "", "v4 SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			image := fs.String("image", v4codex.DefaultImage, "auth runtime image")
			callbackPort := fs.Int("callback-port", v4codex.DefaultCallbackPort, "auth callback relay port")
			callbackTimeout := fs.Duration("callback-timeout", 10*time.Minute, "auth callback relay timeout")
			profileName := fs.String("profile", "", "Profile for this component registration (default: preserve existing or default)")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rt, err := openV4RuntimeForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *image)
			if err != nil {
				return err
			}
			if err := rt.AuthComponent(req.Context(), strings.TrimSpace(req.Params["component"]), v4runtime.AuthOptions{
				Profile:         strings.TrimSpace(*profileName),
				Image:           *image,
				CallbackPort:    *callbackPort,
				CallbackTimeout: *callbackTimeout,
				Stdout:          os.Stdout,
				Stderr:          os.Stderr,
			}); err != nil {
				return err
			}
			fmt.Println("component auth completed")
			return nil
		})

		b.Handle("v4 component list", "List registered v4 components", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v4 component list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v4 state root")
			dbPath := fs.String("db-path", "", "v4 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			rt, err := openV4RuntimeForRoutes(req, store, *stateRoot, *dbPath, "", v4codex.DefaultImage)
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
				profile, err := rt.Profiles.Resolve(componentRow.Profile)
				if err != nil {
					return err
				}
				fmt.Printf("%s\t%s\tprofile=%s\truntime=%s\tdefault=%t\n",
					componentRow.ID,
					componentRow.Ref(),
					componentRow.Profile,
					profile.Runtime,
					componentRow.IsDefault,
				)
			}
			return nil
		})

		b.Handle("v4 chat create <label>", "Create a v4 chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v4 chat create", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v4 state root")
			dbPath := fs.String("db-path", "", "v4 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			rt, err := openV4RuntimeForRoutes(req, store, *stateRoot, *dbPath, "", v4codex.DefaultImage)
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

		b.Handle("v4 chat list", "List v4 chats", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v4 chat list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v4 state root")
			dbPath := fs.String("db-path", "", "v4 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			rt, err := openV4RuntimeForRoutes(req, store, *stateRoot, *dbPath, "", v4codex.DefaultImage)
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

		b.Handle("v4 chat <chatID> component add <role> <component>", "Bind a registered component to a chat by role", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v4 chat component add", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v4 state root")
			dbPath := fs.String("db-path", "", "v4 SQLite DB path")
			externalChatID := fs.String("external-chat-id", "", "External provider chat id for source/relay bindings")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", v4codex.DefaultImage, "Codex runtime image")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			rt, err := openV4RuntimeForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage)
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
				fmt.Printf("profile: %s\n", componentRow.Profile)
			} else {
				fmt.Printf("component_id: %s\n", binding.ComponentID)
			}
			fmt.Printf("role: %s\n", binding.Role)
			if binding.ExternalChatID != "" {
				fmt.Printf("external_chat_id: %s\n", binding.ExternalChatID)
			}
			return nil
		})

		b.Handle("v4 chat <chatID> component list", "List component bindings for a v4 chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("v4 chat component list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "v4 state root")
			dbPath := fs.String("db-path", "", "v4 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			rt, err := openV4RuntimeForRoutes(req, store, *stateRoot, *dbPath, "", v4codex.DefaultImage)
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
				profileName := ""
				if componentRow != nil {
					ref = componentRow.Ref()
					profileName = componentRow.Profile
				}
				fmt.Printf("%s\tprofile=%s\trole=%s\texternal_chat_id=%s\n", ref, profileName, binding.Role, binding.ExternalChatID)
			}
			return nil
		})
	})
}

func openV4RuntimeForRoutes(req *clir.Request, store *clistate.Store, stateRoot string, dbPath string, telegramToken string, codexImage string) (*v4runtime.Runtime, error) {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	rt, err := v4runtime.Open(req.Context(), v4runtime.OpenOptions{
		StateRoot: stateRoot,
		DBPath:    dbPath,
		Store:     store,
		Logger:    logger,
	})
	if err != nil {
		return nil, err
	}
	rt.Registry = newV4Registry(rt, telegramToken, codexImage)
	return rt, nil
}

func newV4Registry(rt *v4runtime.Runtime, telegramToken string, codexImage string) *v4component.Registry {
	return v4component.NewRegistry(
		v4telegram.NewFactory(telegramToken, rt.Config, rt.TelegramUpdates, rt.Logger),
		v4codex.NewFactory(rt.Config, rt.Logger, codexImage),
	)
}

func newV4ProfileManager(store *clistate.Store) (*profiles.Manager, error) {
	rootPath, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	return profiles.New(rootPath, store), nil
}
