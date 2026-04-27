package main

import (
	"context"
	"fmt"
	"strings"

	appstate "github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configengine"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	configschema "github.com/bartdeboer/ctgbot/internal/schema/config"
	"github.com/bartdeboer/ctgbot/internal/schema/routers"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerConfigRoutes(r *clir.Router, store *clistate.Store, globalStore *clistate.Store) {
	var cfg *appstate.Config
	var engine *commandengine.Engine
	var initErr error

	if store == nil {
		initErr = fmt.Errorf("no cwd config store available")
	} else if globalStore == nil {
		initErr = fmt.Errorf("no global config store available")
	} else {
		cfg, initErr = appstate.NewConfig("", store, globalStore)
		if initErr == nil {
			registry, err := configschema.Registry(cfg)
			if err != nil {
				initErr = err
			} else {
				engine, initErr = routers.NewConfigCommandEngine(configengine.New(registry), commandengine.SourceCLI, cliConfigHandlers{cfg: cfg})
			}
		}
	}

	r.Routes(func(b *clir.Builder) {
		b.Handle("config", "Show or update ctgbot config", func(req *clir.Request) error {
			if initErr != nil {
				return initErr
			}
			if len(req.Extra) > 0 {
				return fmt.Errorf("no matching command for `config %s`", joinArgs(req.Extra))
			}
			return printConfigSummary(cfg, store, globalStore)
		})
		b.Handle("config list", "List config items", func(req *clir.Request) error {
			return runCLIConfig(req, initErr, engine, cliConfigRequest(modeluuid.Nil), []string{"config", "list"})
		})
		b.Handle("config get <key>", "Read a root config value", func(req *clir.Request) error {
			return runCLIConfig(req, initErr, engine, cliConfigRequest(modeluuid.Nil), []string{"config", "get", req.Params["key"]})
		})
		b.Handle("config set <key> <value>", "Write a root config value", func(req *clir.Request) error {
			return runCLIConfig(req, initErr, engine, cliConfigRequest(modeluuid.Nil), []string{"config", "set", req.Params["key"], req.Params["value"]})
		})
		b.Handle("config chat <chatID> list", "List config items visible for a chat", func(req *clir.Request) error {
			chatID, err := parseCLIChatID(req.Params["chatID"])
			if err != nil {
				return err
			}
			return runCLIConfig(req, initErr, engine, cliConfigRequest(chatID), []string{"config", "list"})
		})
		b.Handle("config chat <chatID> get <key>", "Read a chat config value", func(req *clir.Request) error {
			chatID, err := parseCLIChatID(req.Params["chatID"])
			if err != nil {
				return err
			}
			return runCLIConfig(req, initErr, engine, cliConfigRequest(chatID), []string{"config", "get", req.Params["key"]})
		})
		b.Handle("config chat <chatID> set <key> <value>", "Write a chat config value", func(req *clir.Request) error {
			chatID, err := parseCLIChatID(req.Params["chatID"])
			if err != nil {
				return err
			}
			return runCLIConfig(req, initErr, engine, cliConfigRequest(chatID), []string{"config", "set", req.Params["key"], req.Params["value"]})
		})
		b.Handle("config chat <chatID> hostbridge scaffold <alias>", "Create an editable hostbridge allowed-command skeleton", func(req *clir.Request) error {
			chatID, err := parseCLIChatID(req.Params["chatID"])
			if err != nil {
				return err
			}
			return runCLIConfig(req, initErr, engine, cliConfigRequest(chatID), []string{"config", "hostbridge", "scaffold", req.Params["alias"]})
		})
	})
}

type cliConfigHandlers struct {
	cfg *appstate.Config
}

func (h cliConfigHandlers) ScaffoldHostbridgeAllowedCommand(_ context.Context, req commandengine.Request, cmd schemacommands.ConfigHostbridgeScaffold) (commandengine.Result, error) {
	if h.cfg == nil {
		return commandengine.Result{}, fmt.Errorf("missing config")
	}
	chatID := req.Context.ChatID
	if chatID.IsNull() {
		return commandengine.Result{}, fmt.Errorf("missing chat id")
	}
	if err := h.cfg.Chat(chatID).Hostbridge().ScaffoldAllowedCommand(cmd.Alias); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("hostbridge allowed command %q scaffolded", cmd.Alias)}, nil
}

func cliConfigRequest(chatID modeluuid.UUID) commandengine.Request {
	return commandengine.Request{Context: commandengine.Context{
		Source: commandengine.SourceCLI,
		Actor:  commandengine.Actor{ID: "cli", Roles: []simplerbac.Role{simplerbac.RoleRoot}},
		ChatID: chatID,
	}}
}

func runCLIConfig(req *clir.Request, initErr error, engine *commandengine.Engine, commandReq commandengine.Request, argv []string) error {
	if initErr != nil {
		return initErr
	}
	if engine == nil {
		return fmt.Errorf("config command engine is not initialized")
	}
	result, err := engine.Run(req.Context(), commandReq, argv)
	if err != nil {
		return err
	}
	if strings.TrimSpace(result.Text) != "" {
		fmt.Println(result.Text)
	}
	return nil
}

func printConfigSummary(cfg *appstate.Config, store *clistate.Store, globalStore *clistate.Store) error {
	if err := cfg.EnsurePaths(); err != nil {
		return err
	}

	fmt.Println("Current config:")
	fmt.Printf("  project_dir: %q\n", globalStore.GetString("project_dir", ""))
	fmt.Printf("  build.compiler_path: %q\n", globalStore.GetString("build.compiler_path", ""))
	fmt.Printf("  telegram.token: %q\n", store.GetString("telegram.token", ""))
	fmt.Printf("  telegram.admin_user_id: %d\n", cfg.Telegram().AdminUserID())
	fmt.Printf("  docker.image: %q\n", cfg.Docker().Image())
	fmt.Printf("  docker.dockerfile: %q\n", cfg.Docker().Dockerfile())
	fmt.Printf("  docker.cli_container_name: %q\n", cfg.Docker().CLIContainerName())
	fmt.Printf("  docker.workspace_host_path: %q\n", cfg.Docker().DefaultWorkspaceHostPath())
	fmt.Printf("  hostbridge.tcp_listen_addr: %q\n", cfg.Hostbridge().TCPListenAddr())
	fmt.Printf("  docker.container_hostbridge_tcp_addr: %q\n", cfg.Docker().ContainerHostbridgeTCPAddr())
	fmt.Printf("  codex.model: %q\n", cfg.Codex().Model())
	fmt.Printf("  codex.profile_host_path: %q\n", cfg.Codex().ProfileHostPath())
	fmt.Printf("  codex.login_callback_port: %d (fixed)\n", appstate.CodexLoginCallbackPort)
	fmt.Printf("  telegram.defaults.poll_timeout_sec: %d\n", int(cfg.Telegram().PollTimeout().Seconds()))
	fmt.Printf("  telegram.defaults.debounce_ms: %d\n", int(cfg.Telegram().DebounceWindow().Milliseconds()))
	fmt.Printf("  telegram.defaults.render_format: %q\n", cfg.Telegram().RenderFormat())
	fmt.Printf("  session.timeout_min: %d\n", int(cfg.Codex().SessionTimeout().Minutes()))
	fmt.Println("  known_chats:")
	chats := cfg.KnownChats()
	if len(chats) == 0 {
		fmt.Println("    <none yet>")
	} else {
		for _, chat := range chats {
			title := chat.ProviderChatTitle
			if title == "" {
				title = "<untitled>"
			}
			fmt.Printf("    - internal_chat_id=%s provider=%s provider_chat_id=%q enabled=%t process_tools=%t gpus=%q title=%q\n", chat.ID.String(), chat.ProviderType, chat.ProviderChatID, chat.Enabled, cfg.Chat(chat.ID).ProcessToolsEnabled(), cfg.Chat(chat.ID).GPUs(), title)
			fmt.Printf("      container_user_mode: %s\n", cfg.Chat(chat.ID).ContainerUserMode())
			workspacePath := cfg.Chat(chat.ID).WorkspaceHostPath()
			if workspacePath == "" {
				workspacePath = "<global/default>"
			}
			fmt.Printf("      workspace_host_path: %s\n", workspacePath)
			fmt.Printf("      codex_profile_host_path: %s\n", cfg.Chat(chat.ID).CodexProfileHostPath())
			skills := cfg.Chat(chat.ID).Skills()
			if len(skills) == 0 {
				fmt.Println("      skills: <none>")
			} else {
				fmt.Printf("      skills: %v\n", skills)
			}
			commands := cfg.Chat(chat.ID).Hostbridge().ConfiguredAllowedCommands()
			if len(commands) == 0 {
				fmt.Println("      hostbridge.allowed_commands: <defaults only>")
				continue
			}
			names := hostbridgeserver.AllowedCommandNames(commands)
			fmt.Printf("      hostbridge.allowed_commands: names=%v\n", names)
			for _, name := range names {
				spec := commands[name]
				fmt.Printf("        %s => name=%q args=%v dir=%q allow_extra_args=%t delay=%q\n", name, spec.Name, spec.Args, spec.Dir, spec.AllowExtraArgs, spec.Delay)
			}
		}
	}
	fmt.Println("  help:")
	fmt.Println("    list root items with: ctgbot config list")
	fmt.Println("    get a root value with: ctgbot config get docker.image")
	fmt.Println("    set a root value with: ctgbot config set docker.image <image>")
	fmt.Println("    set image Dockerfile with: ctgbot config set docker.dockerfile slim.Dockerfile")
	fmt.Println("    list chat-aware items with: ctgbot config chat <chat-id> list")
	fmt.Println("    enable a chat with: ctgbot config chat <chat-id> set chat.enabled true")
	fmt.Println("    set chat process tools with: ctgbot config chat <chat-id> set chat.process-tools-enabled true")
	fmt.Println("    set chat GPUs with: ctgbot config chat <chat-id> set chat.gpus all")
	fmt.Println("    set chat container user mode with: ctgbot config chat <chat-id> set chat.container-user-mode host")
	fmt.Println("    set a chat workspace with: ctgbot config chat <chat-id> set chat.workspace-host-path <path>")
	fmt.Println("    set chat skills with: ctgbot config chat <chat-id> set chat.skills <absolute-skill-dir>[,<absolute-skill-dir>]")
	fmt.Println("    scaffold a chat hostbridge command with: ctgbot config chat <chat-id> hostbridge scaffold <alias>")
	return nil
}

func parseCLIChatID(raw string) (modeluuid.UUID, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return modeluuid.Nil, fmt.Errorf("missing chat id")
	}
	chatID, err := modeluuid.Parse(raw)
	if err != nil {
		return modeluuid.Nil, fmt.Errorf("invalid chat id %q", raw)
	}
	return chatID, nil
}

func joinArgs(args []string) string {
	if len(args) == 0 {
		return ""
	}
	out := args[0]
	for _, arg := range args[1:] {
		out += " " + arg
	}
	return out
}
