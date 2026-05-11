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

	"github.com/bartdeboer/ctgbot/internal/broker"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/codex"
	"github.com/bartdeboer/ctgbot/internal/component/gmail"
	"github.com/bartdeboer/ctgbot/internal/component/llamacpp"
	processcomponent "github.com/bartdeboer/ctgbot/internal/component/process"
	"github.com/bartdeboer/ctgbot/internal/component/telegram"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	inboundguard "github.com/bartdeboer/ctgbot/internal/guard"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	systempkg "github.com/bartdeboer/ctgbot/internal/system"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerRuntimeRoutes(r *clir.Router, store *clistate.Store, globalStore *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("run", "Run the ctgbot runtime", func(req *clir.Request) error {
			fs := flag.NewFlagSet("run", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root (default: <cwd>/.ctgbot)")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", "", "Codex runtime image override")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			runCtx, stop := signal.NotifyContext(req.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			rtSystem, err := openSystemForRoutes(
				req,
				store,
				*stateRoot,
				*dbPath,
				resolveTelegramToken(*telegramToken, store),
				*codexImage,
				newRuntimeProcessActions(globalStore, stop, nil),
			)
			if err != nil {
				return err
			}
			if _, _, err := rtSystem.StartHostbridge(); err != nil {
				return fmt.Errorf("start hostbridge: %w", err)
			}

			fmt.Println("ctgbot runtime initialized")
			fmt.Printf("state_root: %s\n", rtSystem.StateRoot)
			fmt.Printf("database: %s\n", rtSystem.DBPath)

			logf := func(format string, args ...any) {}
			if rtSystem.Logger != nil {
				logf = rtSystem.Logger.Printf
			}
			guardFilter := inboundguard.NewInboundFilter(rtSystem.Storage, rtSystem, logf)
			return broker.New(rtSystem.Storage, rtSystem, logf, guardFilter).Run(runCtx)
		})

		b.Handle("workspace set <workspace>", "Configure a workspace", func(req *clir.Request) error {
			fs := flag.NewFlagSet("workspace set", flag.ContinueOnError)
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

			workspace, err := systempkg.SaveWorkspace(rootDir, store, strings.TrimSpace(req.Params["workspace"]), strings.TrimSpace(*path))
			if err != nil {
				return err
			}
			fmt.Println("workspace saved")
			fmt.Printf("name: %s\n", workspace.Name)
			fmt.Printf("path: %s\n", workspace.Path)
			return nil
		})

		b.Handle("workspace list", "List configured workspaces", func(req *clir.Request) error {
			rootDir, err := filepath.Abs(".")
			if err != nil {
				return err
			}
			workspaces, err := systempkg.LoadWorkspaces(rootDir, store)
			if err != nil {
				return err
			}
			configured := systempkg.ConfiguredWorkspaces(store)
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

		b.Handle("component register <component>", "Register a component instance", func(req *clir.Request) error {
			fs := flag.NewFlagSet("component register", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", "", "Codex runtime image override")
			runtimeKind := fs.String("runtime", "", "Runtime kind for this registered component (docker or local)")
			homePath := fs.String("home", "", "Optional host component home override")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rtSystem, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage, nil)
			if err != nil {
				return err
			}
			registration, err := rtSystem.EnsureComponent(req.Context(), strings.TrimSpace(req.Params["component"]), strings.TrimSpace(*runtimeKind), strings.TrimSpace(*homePath))
			if err != nil {
				return err
			}
			runtime, err := rtSystem.Runtime(registration.Runtime)
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

		b.Handle("component list", "List registered components", func(req *clir.Request) error {
			fs := flag.NewFlagSet("component list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rtSystem, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, "", "", nil)
			if err != nil {
				return err
			}
			components, err := rtSystem.Storage.Components().ListEnabled(req.Context())
			if err != nil {
				return err
			}
			if len(components) == 0 {
				fmt.Println("no registered components")
				return nil
			}
			for _, registration := range components {
				runtime, err := rtSystem.Runtime(registration.Runtime)
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

		b.Handle("component <source> guard set <guard>", "Set the inbound guard component for a source component", func(req *clir.Request) error {
			fs := flag.NewFlagSet("component guard set", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", "", "Codex runtime image override")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rtSystem, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage, nil)
			if err != nil {
				return err
			}
			result, err := setSourceGuard(req.Context(), rtSystem, strings.TrimSpace(req.Params["source"]), strings.TrimSpace(req.Params["guard"]))
			if err != nil {
				return err
			}
			fmt.Println("component guard set")
			fmt.Printf("source: %s\n", result.Source.Ref())
			fmt.Printf("guard: %s\n", result.Guard.Ref())
			fmt.Printf("binding_id: %s\n", result.Binding.ID)
			return nil
		})

		b.Handle("component <source> guard clear", "Clear the inbound guard component for a source component", func(req *clir.Request) error {
			fs := flag.NewFlagSet("component guard clear", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", "", "Codex runtime image override")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rtSystem, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage, nil)
			if err != nil {
				return err
			}
			result, err := clearSourceGuard(req.Context(), rtSystem, strings.TrimSpace(req.Params["source"]))
			if err != nil {
				return err
			}
			fmt.Println("component guard cleared")
			fmt.Printf("source: %s\n", result.Source.Ref())
			fmt.Printf("disabled: %d\n", result.Disabled)
			return nil
		})

		b.Handle("component <source> guard status", "Show the inbound guard component for a source component", func(req *clir.Request) error {
			fs := flag.NewFlagSet("component guard status", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", "", "Codex runtime image override")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rtSystem, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage, nil)
			if err != nil {
				return err
			}
			result, err := sourceGuardStatus(req.Context(), rtSystem, strings.TrimSpace(req.Params["source"]))
			if err != nil {
				return err
			}
			fmt.Println("component guard status")
			fmt.Printf("source: %s\n", result.Source.Ref())
			if len(result.Bindings) == 0 {
				fmt.Println("guard: none")
				return nil
			}
			for _, binding := range result.Bindings {
				fmt.Printf("guard: %s\tbinding_id=%s\n", binding.GuardRef, binding.Binding.ID)
			}
			return nil
		})

		b.Handle("component <component>", "Run a registered component CLI command", func(req *clir.Request) error {
			fs := flag.NewFlagSet("component", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			image := fs.String("image", "", "Runtime image override")
			runtimeKind := fs.String("runtime", "", "Runtime kind for this component registration (used when creating it)")
			homePath := fs.String("home", "", "Optional host component home override")
			callbackPort := fs.Int("callback-port", codex.DefaultCallbackPort, "auth callback relay port")
			callbackTimeout := fs.Duration("callback-timeout", 10*time.Minute, "auth callback relay timeout")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rtSystem, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *image, newRuntimeProcessActions(globalStore, nil, nil))
			if err != nil {
				return err
			}

			componentRef := strings.TrimSpace(req.Params["component"])
			var registration *coremodel.Component
			if strings.TrimSpace(*runtimeKind) != "" || strings.TrimSpace(*homePath) != "" {
				registration, err = rtSystem.EnsureComponent(req.Context(), componentRef, strings.TrimSpace(*runtimeKind), strings.TrimSpace(*homePath))
			} else {
				registration, err = rtSystem.ResolveComponentRef(req.Context(), componentRef)
			}
			if err != nil {
				return err
			}
			argv := fs.Args()
			if len(argv) == 1 && argv[0] == "auth" {
				if *callbackPort != 0 {
					argv = append(argv, "--callback-port", fmt.Sprintf("%d", *callbackPort))
				}
				if *callbackTimeout != 0 {
					argv = append(argv, "--callback-timeout", callbackTimeout.String())
				}
			}
			return runComponentCLI(req, rtSystem, registration, argv)
		})

		b.Handle("source <source> guard set <guard>", "Set the inbound guard component for a source component", func(req *clir.Request) error {
			fs := flag.NewFlagSet("source guard set", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", "", "Codex runtime image override")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rtSystem, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage, nil)
			if err != nil {
				return err
			}
			result, err := setSourceGuard(req.Context(), rtSystem, strings.TrimSpace(req.Params["source"]), strings.TrimSpace(req.Params["guard"]))
			if err != nil {
				return err
			}
			fmt.Println("source guard set")
			fmt.Printf("source: %s\n", result.Source.Ref())
			fmt.Printf("guard: %s\n", result.Guard.Ref())
			fmt.Printf("binding_id: %s\n", result.Binding.ID)
			return nil
		}, clir.Hidden())

		b.Handle("source <source> guard clear", "Clear the inbound guard component for a source component", func(req *clir.Request) error {
			fs := flag.NewFlagSet("source guard clear", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", "", "Codex runtime image override")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rtSystem, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage, nil)
			if err != nil {
				return err
			}
			result, err := clearSourceGuard(req.Context(), rtSystem, strings.TrimSpace(req.Params["source"]))
			if err != nil {
				return err
			}
			fmt.Println("source guard cleared")
			fmt.Printf("source: %s\n", result.Source.Ref())
			fmt.Printf("disabled: %d\n", result.Disabled)
			return nil
		}, clir.Hidden())

		b.Handle("chat create <label>", "Create a chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chat create", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, "", "", nil)
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

		b.Handle("chat list", "List chats", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chat list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, "", "", nil)
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

		b.Handle("chat dropped", "List unresolved dropped inbound chats", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chat dropped", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, "", "", nil)
			if err != nil {
				return err
			}
			drops, err := system.Storage.InboundDrops().List(req.Context())
			if err != nil {
				return err
			}
			if len(drops) == 0 {
				fmt.Println("no dropped chats")
				return nil
			}
			for _, drop := range drops {
				ref := drop.ComponentID.String()
				registration, err := system.Storage.Components().GetByID(req.Context(), drop.ComponentID)
				if err != nil {
					return err
				}
				if registration != nil {
					ref = registration.Ref()
				}
				fmt.Printf("%s\texternal_chat_id=%s\tmessages=%d\tlast_seen=%s\tlabel=%s\tactor=%s\tpreview=%s\n",
					ref,
					drop.ExternalChatID,
					drop.MessageCount,
					drop.LastSeenAt.Format(time.RFC3339),
					drop.ChatLabel,
					displayActor(drop.ActorLabel, drop.ActorID),
					drop.LastTextPreview,
				)
			}
			return nil
		})

		b.Handle("chat bind <component> <externalChatID>", "Create an enabled chat for a dropped inbound external chat and bind the inbound component", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chat bind", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			roleFlag := fs.String("role", "", "Binding role override (source, relay, or all)")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), "", nil)
			if err != nil {
				return err
			}
			componentRef := strings.TrimSpace(req.Params["component"])
			externalChatID := strings.TrimSpace(req.Params["externalChatID"])
			if externalChatID == "" {
				return fmt.Errorf("missing external chat id")
			}
			registration, err := system.ResolveComponentRef(req.Context(), componentRef)
			if err != nil {
				return err
			}
			loaded, err := system.ResolveComponent(req.Context(), registration.ID)
			if err != nil {
				return err
			}
			roles, err := resolveChatBindRoles(loaded, strings.TrimSpace(*roleFlag))
			if err != nil {
				return err
			}
			label := strings.TrimSpace(strings.Join(fs.Args(), " "))
			drop, err := system.Storage.InboundDrops().GetByComponentAndExternalChatID(req.Context(), registration.ID, externalChatID)
			if err != nil {
				return err
			}
			if label == "" && drop != nil {
				label = strings.TrimSpace(drop.ChatLabel)
			}
			if label == "" {
				label = externalChatID
			}

			chat, bindings, err := bindInboundChat(req.Context(), system.Storage, *registration, externalChatID, label, roles)
			if err != nil {
				return err
			}
			fmt.Println("chat bound")
			fmt.Printf("chat_id: %s\n", chat.ID)
			fmt.Printf("label: %s\n", chat.Label)
			for _, binding := range bindings {
				fmt.Printf("binding: role=%s component=%s external_chat_id=%s\n", binding.Role, registration.Ref(), binding.ExternalChatID)
			}
			return nil
		})

		b.Handle("chat <chatID> workspace set <workspace>", "Assign a named workspace to a chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chat workspace set", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, "", "", nil)
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

		b.Handle("chat <chatID> workspace clear", "Clear the named workspace from a chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chat workspace clear", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, "", "", nil)
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

		b.Handle("chat <chatID> component add <role> <component>", "Bind a registered component to a chat by role", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chat component add", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			externalChatID := fs.String("external-chat-id", "", "External provider chat id for source/relay bindings")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexImage := fs.String("codex-image", "", "Codex runtime image override")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, resolveTelegramToken(*telegramToken, store), *codexImage, nil)
			if err != nil {
				return err
			}
			chatID, err := modeluuid.Parse(strings.TrimSpace(req.Params["chatID"]))
			if err != nil {
				return fmt.Errorf("parse chat id: %w", err)
			}
			role := coremodel.ChatComponentRole(strings.TrimSpace(req.Params["role"]))
			componentRef := strings.TrimSpace(req.Params["component"])
			externalChatIDValue := strings.TrimSpace(*externalChatID)
			if externalChatIDValue == "" && role == coremodel.ChatComponentRoleSource {
				externalChatIDValue, err = defaultSourceExternalChatID(req.Context(), system, componentRef)
				if err != nil {
					return err
				}
			}
			binding, err := system.BindChatComponent(req.Context(), chatID, role, componentRef, externalChatIDValue)
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

		b.Handle("chat <chatID> component list", "List component bindings for a chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chat component list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			stateRoot := fs.String("state-root", "", "ctgbot state root")
			dbPath := fs.String("db-path", "", "SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			system, err := openSystemForRoutes(req, store, *stateRoot, *dbPath, "", "", nil)
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

func defaultSourceExternalChatID(ctx context.Context, system *systempkg.System, componentRef string) (string, error) {
	if system == nil {
		return "", fmt.Errorf("missing system")
	}
	registration, err := system.ResolveComponentRef(ctx, componentRef)
	if err != nil {
		return "", err
	}
	loaded, err := system.ResolveComponent(ctx, registration.ID)
	if err != nil {
		return "", err
	}
	defaults, ok := loaded.Component.(component.SourceBindingDefaults)
	if !ok {
		return "", nil
	}
	return defaults.DefaultSourceExternalChatID(ctx)
}

type sourceGuardSetResult struct {
	Source  coremodel.Component
	Guard   coremodel.Component
	Binding coremodel.ComponentBinding
}

type sourceGuardClearResult struct {
	Source   coremodel.Component
	Disabled int
}

type sourceGuardStatusResult struct {
	Source   coremodel.Component
	Bindings []sourceGuardStatusBinding
}

type sourceGuardStatusBinding struct {
	Binding  coremodel.ComponentBinding
	GuardRef string
}

func setSourceGuard(ctx context.Context, system *systempkg.System, sourceRef string, guardRef string) (sourceGuardSetResult, error) {
	if system == nil || system.Storage == nil {
		return sourceGuardSetResult{}, fmt.Errorf("missing system storage")
	}
	source, err := resolveInboundSourceRegistration(ctx, system, sourceRef)
	if err != nil {
		return sourceGuardSetResult{}, err
	}
	guard, err := resolveCompletionProviderRegistration(ctx, system, guardRef)
	if err != nil {
		return sourceGuardSetResult{}, err
	}

	var binding coremodel.ComponentBinding
	if err := system.Storage.Transaction(ctx, func(tx repository.Storage) error {
		existing, err := tx.ComponentBindings().ListEnabledBySourceAndRole(ctx, source.ID, coremodel.ComponentBindingRoleGuard)
		if err != nil {
			return err
		}
		for _, old := range existing {
			old.Enabled = false
			if err := tx.ComponentBindings().Save(ctx, &old); err != nil {
				return err
			}
		}

		current, err := tx.ComponentBindings().GetBySourceTargetRole(ctx, source.ID, guard.ID, coremodel.ComponentBindingRoleGuard)
		if err != nil {
			return err
		}
		if current != nil {
			binding = *current
		} else {
			binding = coremodel.ComponentBinding{
				SourceComponentID: source.ID,
				TargetComponentID: guard.ID,
				Role:              coremodel.ComponentBindingRoleGuard,
			}
		}
		binding.Enabled = true
		return tx.ComponentBindings().Save(ctx, &binding)
	}); err != nil {
		return sourceGuardSetResult{}, err
	}

	return sourceGuardSetResult{Source: *source, Guard: *guard, Binding: binding}, nil
}

func clearSourceGuard(ctx context.Context, system *systempkg.System, sourceRef string) (sourceGuardClearResult, error) {
	if system == nil || system.Storage == nil {
		return sourceGuardClearResult{}, fmt.Errorf("missing system storage")
	}
	source, err := resolveInboundSourceRegistration(ctx, system, sourceRef)
	if err != nil {
		return sourceGuardClearResult{}, err
	}

	disabled := 0
	if err := system.Storage.Transaction(ctx, func(tx repository.Storage) error {
		existing, err := tx.ComponentBindings().ListEnabledBySourceAndRole(ctx, source.ID, coremodel.ComponentBindingRoleGuard)
		if err != nil {
			return err
		}
		for _, binding := range existing {
			binding.Enabled = false
			if err := tx.ComponentBindings().Save(ctx, &binding); err != nil {
				return err
			}
			disabled++
		}
		return nil
	}); err != nil {
		return sourceGuardClearResult{}, err
	}

	return sourceGuardClearResult{Source: *source, Disabled: disabled}, nil
}

func sourceGuardStatus(ctx context.Context, system *systempkg.System, sourceRef string) (sourceGuardStatusResult, error) {
	if system == nil || system.Storage == nil {
		return sourceGuardStatusResult{}, fmt.Errorf("missing system storage")
	}
	source, err := resolveInboundSourceRegistration(ctx, system, sourceRef)
	if err != nil {
		return sourceGuardStatusResult{}, err
	}
	bindings, err := system.Storage.ComponentBindings().ListEnabledBySourceAndRole(ctx, source.ID, coremodel.ComponentBindingRoleGuard)
	if err != nil {
		return sourceGuardStatusResult{}, err
	}
	result := sourceGuardStatusResult{
		Source:   *source,
		Bindings: make([]sourceGuardStatusBinding, 0, len(bindings)),
	}
	for _, binding := range bindings {
		guardRef := binding.TargetComponentID.String()
		registration, err := system.Storage.Components().GetByID(ctx, binding.TargetComponentID)
		if err != nil {
			return sourceGuardStatusResult{}, err
		}
		if registration != nil {
			guardRef = registration.Ref()
		}
		result.Bindings = append(result.Bindings, sourceGuardStatusBinding{
			Binding:  binding,
			GuardRef: guardRef,
		})
	}
	return result, nil
}

func resolveInboundSourceRegistration(ctx context.Context, system *systempkg.System, ref string) (*coremodel.Component, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("missing source component ref")
	}
	registration, err := system.ResolveComponentRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	loaded, err := system.ResolveComponent(ctx, registration.ID)
	if err != nil {
		return nil, err
	}
	if _, ok := loaded.Component.(component.InboundSource); !ok {
		return nil, fmt.Errorf("component %s does not support inbound source", registration.Ref())
	}
	return registration, nil
}

func resolveCompletionProviderRegistration(ctx context.Context, system *systempkg.System, ref string) (*coremodel.Component, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("missing guard component ref")
	}
	registration, err := system.ResolveComponentRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	loaded, err := system.ResolveComponent(ctx, registration.ID)
	if err != nil {
		return nil, err
	}
	if _, ok := loaded.Component.(component.CompletionProvider); !ok {
		return nil, fmt.Errorf("component %s does not support completion provider guard", registration.Ref())
	}
	return registration, nil
}

func runComponentCLI(req *clir.Request, system *systempkg.System, registration *coremodel.Component, argv []string) error {
	if req == nil {
		return fmt.Errorf("missing request")
	}
	if system == nil {
		return fmt.Errorf("missing system")
	}
	if registration == nil {
		return fmt.Errorf("missing component registration")
	}
	loaded, err := system.ResolveComponent(req.Context(), registration.ID)
	if err != nil {
		return err
	}
	bound := boundCLIComponentSurfaces(loaded)
	if len(bound) == 0 {
		return fmt.Errorf("component has no CLI commands: %s", registration.Ref())
	}
	definitions := commandset.DefinitionsForBoundSource(commandengine.SourceCLI, bound)
	if len(argv) == 0 {
		printComponentCLIHelp(definitions)
		return nil
	}
	engine, err := commandset.NewBoundEngineForSource(commandengine.SourceCLI, bound)
	if err != nil {
		return err
	}
	base := commandengine.Request{
		Context: commandengine.Context{
			Source: commandengine.SourceCLI,
			Actor: commandengine.Actor{
				ID:    "cli",
				Roles: []simplerbac.Role{simplerbac.RoleRoot},
			},
		},
	}
	result, err := engine.Run(req.Context(), base, append([]string{registration.Ref()}, argv...))
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.Text) != "" {
		fmt.Println(result.Text)
	}
	return nil
}

func boundCLIComponentSurfaces(loaded *component.Loaded) []commandset.BoundSurface {
	if loaded == nil || loaded.Component == nil {
		return nil
	}
	componentRef := loaded.Registration.Ref()
	componentType := strings.TrimSpace(loaded.Registration.Type)
	var bound []commandset.BoundSurface
	if surface, ok := loaded.Component.(component.CommandSurface); ok {
		bound = append(bound, commandset.BoundSurface{
			Surface:       surface,
			ComponentRef:  componentRef,
			ComponentType: componentType,
		})
	}
	if surface := component.NewCLIAdminSurface(loaded.Component); surface != nil {
		bound = append(bound, commandset.BoundSurface{
			Surface:       surface,
			ComponentRef:  componentRef,
			ComponentType: componentType,
		})
	}
	return bound
}

func printComponentCLIHelp(definitions []commandengine.Definition) {
	patterns := commandset.CanonicalRoutePatterns(definitions, coremodel.Actor{
		Roles: []simplerbac.Role{simplerbac.RoleRoot},
	})
	if len(patterns) == 0 {
		fmt.Println("no component CLI commands")
		return
	}
	fmt.Println("available component commands:")
	for _, pattern := range patterns {
		fmt.Printf("  %s\n", pattern)
	}
}

func resolveChatBindRoles(loaded *component.Loaded, roleFlag string) ([]coremodel.ChatComponentRole, error) {
	if loaded == nil || loaded.Component == nil {
		return nil, fmt.Errorf("missing loaded component")
	}
	_, hasSource := loaded.Component.(component.InboundSource)
	_, hasRelay := loaded.Component.(component.OutboundRelay)

	switch strings.TrimSpace(roleFlag) {
	case "", "auto":
		switch {
		case hasSource && hasRelay:
			return []coremodel.ChatComponentRole{coremodel.ChatComponentRoleSource, coremodel.ChatComponentRoleRelay}, nil
		case hasSource:
			return []coremodel.ChatComponentRole{coremodel.ChatComponentRoleSource}, nil
		case hasRelay:
			return []coremodel.ChatComponentRole{coremodel.ChatComponentRoleRelay}, nil
		default:
			return nil, fmt.Errorf("component %s does not support source or relay chat bindings", loaded.Registration.Ref())
		}
	case string(coremodel.ChatComponentRoleSource):
		if !hasSource {
			return nil, fmt.Errorf("component %s does not support source chat bindings", loaded.Registration.Ref())
		}
		return []coremodel.ChatComponentRole{coremodel.ChatComponentRoleSource}, nil
	case string(coremodel.ChatComponentRoleRelay):
		if !hasRelay {
			return nil, fmt.Errorf("component %s does not support relay chat bindings", loaded.Registration.Ref())
		}
		return []coremodel.ChatComponentRole{coremodel.ChatComponentRoleRelay}, nil
	case "all":
		if !hasSource || !hasRelay {
			return nil, fmt.Errorf("component %s does not support binding both source and relay roles", loaded.Registration.Ref())
		}
		return []coremodel.ChatComponentRole{coremodel.ChatComponentRoleSource, coremodel.ChatComponentRoleRelay}, nil
	default:
		return nil, fmt.Errorf("invalid bind role %q", roleFlag)
	}
}

func bindInboundChat(ctx context.Context, storage repository.Storage, registration coremodel.Component, externalChatID string, label string, roles []coremodel.ChatComponentRole) (*coremodel.Chat, []coremodel.ChatComponent, error) {
	if storage == nil {
		return nil, nil, fmt.Errorf("missing storage")
	}
	externalChatID = strings.TrimSpace(externalChatID)
	label = strings.TrimSpace(label)
	if externalChatID == "" {
		return nil, nil, fmt.Errorf("missing external chat id")
	}
	if label == "" {
		label = externalChatID
	}
	if len(roles) == 0 {
		return nil, nil, fmt.Errorf("missing chat bind roles")
	}

	var chat coremodel.Chat
	var bindings []coremodel.ChatComponent
	err := storage.Transaction(ctx, func(tx repository.Storage) error {
		for _, role := range roles {
			existing, err := tx.ChatComponents().FindByComponentRoleAndExternalChatID(ctx, registration.ID, role, externalChatID)
			if err != nil {
				return err
			}
			if existing != nil {
				return fmt.Errorf("external chat %q is already bound to chat %s as %s", externalChatID, existing.ChatID, role)
			}
		}

		chat = coremodel.Chat{
			Label:   label,
			Enabled: true,
		}
		if err := tx.Chats().Save(ctx, &chat); err != nil {
			return err
		}
		bindings = make([]coremodel.ChatComponent, 0, len(roles))
		for _, role := range roles {
			binding := coremodel.ChatComponent{
				ChatID:         chat.ID,
				ComponentID:    registration.ID,
				Role:           role,
				ExternalChatID: externalChatID,
				Enabled:        true,
			}
			if err := tx.ChatComponents().Save(ctx, &binding); err != nil {
				return err
			}
			bindings = append(bindings, binding)
		}
		return tx.InboundDrops().DeleteByComponentAndExternalChatID(ctx, registration.ID, externalChatID)
	})
	if err != nil {
		return nil, nil, err
	}
	return &chat, bindings, nil
}

func displayActor(label string, id string) string {
	label = strings.TrimSpace(label)
	id = strings.TrimSpace(id)
	switch {
	case label != "" && id != "" && label != id:
		return label + " (" + id + ")"
	case label != "":
		return label
	default:
		return id
	}
}

func openSystemForRoutes(req *clir.Request, store *clistate.Store, stateRoot string, dbPath string, telegramToken string, codexImage string, processActions processcomponent.Actions) (*systempkg.System, error) {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	rtSystem, err := systempkg.Open(req.Context(), stateRoot, dbPath, store, logger)
	if err != nil {
		return nil, err
	}
	rtSystem.Registry, err = newRuntimeRegistry(rtSystem, telegramToken, codexImage, processActions)
	if err != nil {
		return nil, err
	}
	return rtSystem, nil
}

func newRuntimeRegistry(rtSystem *systempkg.System, telegramToken string, codexImage string, processActions processcomponent.Actions) (*component.Registry, error) {
	registry := component.NewRegistry()

	if err := registry.Add(telegram.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return telegram.New(ctx, registration, runtime, home, storage, telegramToken, rtSystem.Config, rtSystem.Logger)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(codex.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return codex.New(ctx, registration, runtime, home, storage, rtSystem.Config, rtSystem.ResolveChatWorkspace, rtSystem.Logger, strings.TrimSpace(codexImage))
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(gmail.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return gmail.NewWithOptions(ctx, registration, runtime, home, storage, gmail.Options{OAuthClientConfigPath: filepath.Join(rtSystem.StateRoot, "google", "oauth_client.json"), Logger: rtSystem.Logger})
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(llamacpp.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return llamacpp.New(ctx, registration, runtime, home, storage, rtSystem.Logger)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(processcomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _, _ = ctx, registration, runtime, home, storage
		return processcomponent.New(processActions), nil
	}); err != nil {
		return nil, err
	}
	return registry, nil
}
