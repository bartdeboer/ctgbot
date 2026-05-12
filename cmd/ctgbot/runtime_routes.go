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

	"github.com/bartdeboer/ctgbot/internal/app"
	"github.com/bartdeboer/ctgbot/internal/broker"
	"github.com/bartdeboer/ctgbot/internal/buildassets"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/claude"
	"github.com/bartdeboer/ctgbot/internal/component/codex"
	"github.com/bartdeboer/ctgbot/internal/component/gmail"
	"github.com/bartdeboer/ctgbot/internal/component/llamacpp"
	processcomponent "github.com/bartdeboer/ctgbot/internal/component/process"
	sqlcomponent "github.com/bartdeboer/ctgbot/internal/component/sql"
	"github.com/bartdeboer/ctgbot/internal/component/telegram"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	systempkg "github.com/bartdeboer/ctgbot/internal/system"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerRuntimeRoutes(r *clir.Router, store *clistate.Store, globalStore *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("run", "Run the ctgbot runtime", func(req *clir.Request) error {
			fs := flag.NewFlagSet("run", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			runCtx, stop := signal.NotifyContext(req.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			rtSystem, err := openSystemForRoutes(req, store, newRuntimeProcessActions(globalStore, stop, nil))
			if err != nil {
				return err
			}
			if _, _, err := rtSystem.StartHostbridge(); err != nil {
				return fmt.Errorf("start hostbridge: %w", err)
			}

			fmt.Println("ctgbot runtime initialized")
			fmt.Printf("ctgbot_version: %s\n", buildassets.Version())
			fmt.Printf("state_root: %s\n", rtSystem.StateRoot)
			fmt.Printf("database: %s\n", rtSystem.DBPath)

			logf := func(format string, args ...any) {}
			if rtSystem.Logger != nil {
				logf = rtSystem.Logger.Printf
			}
			appService := app.NewServiceWithLogger(rtSystem.Storage, rtSystem, logf)
			return broker.New(appService, logf).Run(runCtx)
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
			runtimeKind := fs.String("runtime", "", "Runtime kind for this registered component (docker or local)")
			homePath := fs.String("home", "", "Optional host component home override")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			result, err := appService.RegisterComponent(req.Context(), strings.TrimSpace(req.Params["component"]), strings.TrimSpace(*runtimeKind), strings.TrimSpace(*homePath))
			if err != nil {
				return err
			}
			registration := result.Component

			fmt.Println("component registered")
			fmt.Printf("id: %s\n", registration.ID)
			fmt.Printf("ref: %s\n", registration.Ref())
			fmt.Printf("runtime: %s\n", registration.Runtime)
			fmt.Printf("home_path: %s\n", registration.HomePath)
			fmt.Printf("host_home: %s\n", result.HostHomePath)
			fmt.Printf("runtime_home: %s\n", result.RuntimeHomePath)
			return nil
		})

		b.Handle("component list", "List registered components", func(req *clir.Request) error {
			fs := flag.NewFlagSet("component list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			components, err := appService.ListComponents(req.Context())
			if err != nil {
				return err
			}
			if len(components) == 0 {
				fmt.Println("no registered components")
				return nil
			}
			for _, component := range components {
				registration := component.Component
				fmt.Printf("%s\t%s\truntime=%s\tdefault=%t\n",
					registration.ID,
					registration.Ref(),
					component.RuntimeKind,
					registration.IsDefault,
				)
				fmt.Printf("\thost_home=%s\thome_path=%s\n", component.HostHomePath, registration.HomePath)
			}
			return nil
		})

		b.Handle("component <source> guard set <guard>", "Set the inbound guard component for a source component", func(req *clir.Request) error {
			fs := flag.NewFlagSet("component guard set", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			result, err := appService.SetComponentGuard(req.Context(), strings.TrimSpace(req.Params["source"]), strings.TrimSpace(req.Params["guard"]))
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
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			result, err := appService.ClearComponentGuard(req.Context(), strings.TrimSpace(req.Params["source"]))
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
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			result, err := appService.ComponentGuardStatus(req.Context(), strings.TrimSpace(req.Params["source"]))
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
			runtimeKind := fs.String("runtime", "", "Runtime kind for this component registration (used when creating it)")
			homePath := fs.String("home", "", "Optional host component home override")
			callbackPort := fs.Int("callback-port", codex.DefaultCallbackPort, "auth callback relay port")
			callbackTimeout := fs.Duration("callback-timeout", 10*time.Minute, "auth callback relay timeout")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutesWithProcessActions(req, store, newRuntimeProcessActions(globalStore, nil, nil))
			if err != nil {
				return err
			}

			componentRef := strings.TrimSpace(req.Params["component"])
			argv := fs.Args()
			if len(argv) == 1 && argv[0] == "auth" {
				if *callbackPort != 0 {
					argv = append(argv, "--callback-port", fmt.Sprintf("%d", *callbackPort))
				}
				if *callbackTimeout != 0 {
					argv = append(argv, "--callback-timeout", callbackTimeout.String())
				}
			}
			result, err := appService.RunComponentCommand(req.Context(), app.ComponentCommandRequest{
				ComponentRef: componentRef,
				RuntimeKind:  strings.TrimSpace(*runtimeKind),
				HomePath:     strings.TrimSpace(*homePath),
				Args:         argv,
			})
			if err != nil {
				return err
			}
			if strings.TrimSpace(result.Text) != "" {
				fmt.Println(result.Text)
			}
			return nil
		})

		b.Handle("source <source> guard set <guard>", "Set the inbound guard component for a source component", func(req *clir.Request) error {
			fs := flag.NewFlagSet("source guard set", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			result, err := appService.SetComponentGuard(req.Context(), strings.TrimSpace(req.Params["source"]), strings.TrimSpace(req.Params["guard"]))
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
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			result, err := appService.ClearComponentGuard(req.Context(), strings.TrimSpace(req.Params["source"]))
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
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			chat, err := appService.CreateChat(req.Context(), strings.TrimSpace(req.Params["label"]), "")
			if err != nil {
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
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			chats, err := appService.ListChats(req.Context())
			if err != nil {
				return err
			}
			if len(chats) == 0 {
				fmt.Println("no chats")
				return nil
			}
			for _, info := range chats {
				chat := info.Chat
				fmt.Printf("%s\tshort_id=%s\t%s\tworkspace=%s\tenabled=%t\n", chat.ID, info.ShortID, chat.Label, chat.Workspace, chat.Enabled)
			}
			return nil
		})

		b.Handle("chat dropped", "List unresolved dropped inbound chats", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chat dropped", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			drops, err := appService.ListInboundDrops(req.Context())
			if err != nil {
				return err
			}
			if len(drops) == 0 {
				fmt.Println("no dropped chats")
				return nil
			}
			for _, drop := range drops {
				fmt.Printf("%s\texternal_channel_id=%s\tmessages=%d\tlast_seen=%s\tlabel=%s\tactor=%s\tpreview=%s\n",
					drop.ComponentRef,
					drop.ExternalChannelID,
					drop.MessageCount,
					drop.LastSeenAt.Format(time.RFC3339),
					drop.ChatLabel,
					displayActor(drop.ActorLabel, drop.ActorID),
					drop.LastTextPreview,
				)
			}
			return nil
		})

		b.Handle("chat bind <component> <externalChannelID>", "Create an enabled chat for a dropped inbound external channel and bind the inbound component", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chat bind", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			roleFlag := fs.String("role", "", "Binding role override (source, relay, or all)")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			componentRef := strings.TrimSpace(req.Params["component"])
			externalChannelID := strings.TrimSpace(req.Params["externalChannelID"])
			label := strings.TrimSpace(strings.Join(fs.Args(), " "))
			result, err := appService.BindInboundChat(req.Context(), componentRef, externalChannelID, label, strings.TrimSpace(*roleFlag))
			if err != nil {
				return err
			}
			fmt.Println("chat bound")
			fmt.Printf("chat_id: %s\n", result.Chat.ID)
			fmt.Printf("label: %s\n", result.Chat.Label)
			for _, binding := range result.Bindings {
				fmt.Printf("binding: role=%s component=%s external_channel_id=%s\n", binding.Role, result.Component.Ref(), binding.ExternalChannelID)
			}
			return nil
		})

		b.Handle("chat <chatID> workspace set <workspace>", "Assign a named workspace to a chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chat workspace set", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			chatID, err := appService.ResolveChatRef(req.Context(), strings.TrimSpace(req.Params["chatID"]))
			if err != nil {
				return fmt.Errorf("resolve chat id: %w", err)
			}
			chat, err := appService.SetChatWorkspace(req.Context(), chatID, strings.TrimSpace(req.Params["workspace"]))
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
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			chatID, err := appService.ResolveChatRef(req.Context(), strings.TrimSpace(req.Params["chatID"]))
			if err != nil {
				return fmt.Errorf("resolve chat id: %w", err)
			}
			chat, err := appService.SetChatWorkspace(req.Context(), chatID, "")
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
			externalChannelID := fs.String("external-channel-id", "", "External provider channel id for source/relay bindings")
			externalChatID := fs.String("external-chat-id", "", "Deprecated alias for --external-channel-id")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			externalChannelIDValue := strings.TrimSpace(*externalChannelID)
			if externalChannelIDValue == "" {
				externalChannelIDValue = strings.TrimSpace(*externalChatID)
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			chatID, err := appService.ResolveChatRef(req.Context(), strings.TrimSpace(req.Params["chatID"]))
			if err != nil {
				return fmt.Errorf("resolve chat id: %w", err)
			}
			role := coremodel.ChatComponentRole(strings.TrimSpace(req.Params["role"]))
			componentRef := strings.TrimSpace(req.Params["component"])
			result, err := appService.AddChatComponent(req.Context(), chatID, role, componentRef, externalChannelIDValue)
			if err != nil {
				return err
			}
			fmt.Println("chat component bound")
			fmt.Printf("chat_id: %s\n", result.Binding.ChatID)
			if result.ComponentRef != "" {
				fmt.Printf("component: %s\n", result.ComponentRef)
				fmt.Printf("runtime: %s\n", result.Runtime)
				fmt.Printf("home_path: %s\n", result.HomePath)
			} else {
				fmt.Printf("component_id: %s\n", result.Binding.ComponentID)
			}
			fmt.Printf("role: %s\n", result.Binding.Role)
			if result.Binding.ExternalChannelID != "" {
				fmt.Printf("external_channel_id: %s\n", result.Binding.ExternalChannelID)
			}
			return nil
		})

		b.Handle("chat <chatID> component list", "List component bindings for a chat", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chat component list", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			appService, err := openAppServiceForRoutes(req, store)
			if err != nil {
				return err
			}
			chatID, err := appService.ResolveChatRef(req.Context(), strings.TrimSpace(req.Params["chatID"]))
			if err != nil {
				return fmt.Errorf("resolve chat id: %w", err)
			}
			bindings, err := appService.ListChatComponents(req.Context(), chatID)
			if err != nil {
				return err
			}
			if len(bindings) == 0 {
				fmt.Println("no component bindings")
				return nil
			}
			for _, binding := range bindings {
				fmt.Printf("%s\truntime=%s\trole=%s\texternal_channel_id=%s\n", binding.ComponentRef, binding.Runtime, binding.Binding.Role, binding.Binding.ExternalChannelID)
			}
			return nil
		})
	})
}

func openAppServiceForRoutes(req *clir.Request, store *clistate.Store) (*app.Service, error) {
	return openAppServiceForRoutesWithProcessActions(req, store, nil)
}

func openAppServiceForRoutesWithProcessActions(req *clir.Request, store *clistate.Store, processActions processcomponent.Actions) (*app.Service, error) {
	system, err := openSystemForRoutes(req, store, processActions)
	if err != nil {
		return nil, err
	}
	return app.NewService(system.Storage, system), nil
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

func openSystemForRoutes(req *clir.Request, store *clistate.Store, processActions processcomponent.Actions) (*systempkg.System, error) {
	logger := log.New(os.Stdout, "", log.LstdFlags)
	rtSystem, err := systempkg.Open(req.Context(), "", "", store, logger)
	if err != nil {
		return nil, err
	}
	rtSystem.Registry, err = newRuntimeRegistry(rtSystem, processActions)
	if err != nil {
		return nil, err
	}
	return rtSystem, nil
}

func newRuntimeRegistry(rtSystem *systempkg.System, processActions processcomponent.Actions) (*component.Registry, error) {
	registry := component.NewRegistry()

	if err := registry.Add(telegram.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return telegram.New(ctx, registration, runtime, home, storage, rtSystem.Logger)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(codex.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return codex.New(ctx, registration, runtime, home, storage, rtSystem.Config, rtSystem.ResolveChatWorkspace, rtSystem.Logger, "")
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(claude.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return claude.New(ctx, registration, runtime, home, storage, rtSystem.ResolveChatWorkspace, rtSystem.Logger)
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
	if err := registry.Add(sqlcomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _ = ctx, registration, runtime, home
		return sqlcomponent.New(rtSystem.DB)
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
