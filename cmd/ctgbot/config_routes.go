package main

import (
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/clisetter"
	"github.com/bartdeboer/ctgbot/internal/configsetters"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerConfigRoutes(r *clir.Router, store *clistate.Store, globalStore *clistate.Store) {
	var cfg *appstate.Config
	var setter *clisetter.Setter
	var initErr error

	if store == nil {
		initErr = fmt.Errorf("no cwd config store available")
	} else if globalStore == nil {
		initErr = fmt.Errorf("no global config store available")
	} else {
		cfg, initErr = appstate.NewConfig("", store)
		if initErr == nil {
			target := configsetters.NewConfigSetters(cfg, store, globalStore)
			setter = clisetter.New(target)
		}
	}

	r.Routes(func(b *clir.Builder) {
		b.Handle("config", "Show or update ctgbot config", func(req *clir.Request) error {
			if initErr != nil {
				return initErr
			}

			applied, err := setter.HandleRoot(req.Extra)
			if err != nil {
				return err
			}
			if applied {
				fmt.Println("config updated")
				return nil
			}
			if len(req.Extra) > 0 {
				return fmt.Errorf("no matching command for `config %s`", joinArgs(req.Extra))
			}

			if err := cfg.EnsurePaths(); err != nil {
				return err
			}

			fmt.Println("Current config:")
			fmt.Printf("  project_dir: %q\n", globalStore.GetString("project_dir", ""))
			fmt.Printf("  build.compiler_path: %q\n", globalStore.GetString("build.compiler_path", ""))
			fmt.Printf("  telegram.token: %q\n", store.GetString("telegram.token", ""))
			fmt.Printf("  telegram.admin_user_id: %d\n", cfg.TelegramAdminUserID())
			fmt.Printf("  docker.image: %q\n", cfg.DockerImage())
			fmt.Printf("  docker.cli_container_name: %q\n", cfg.DockerCLIContainerName())
			fmt.Printf("  docker.workspace_host_path: %q\n", cfg.DefaultWorkspaceHostPath())
			fmt.Printf("  hostbridge.tcp_listen_addr: %q\n", cfg.HostbridgeTCPListenAddr())
			fmt.Printf("  docker.container_hostbridge_tcp_addr: %q\n", cfg.ContainerHostbridgeTCPAddr())
			fmt.Printf("  codex.model: %q\n", cfg.CodexModel())
			fmt.Printf("  codex.cli_home_host_path: %q\n", cfg.CodexCLIHomeRoot())
			fmt.Printf("  codex.login_callback_port: %d (fixed)\n", appstate.CodexLoginCallbackPort)
			fmt.Printf("  telegram.defaults.poll_timeout_sec: %d\n", int(cfg.PollTimeout().Seconds()))
			fmt.Printf("  telegram.defaults.debounce_ms: %d\n", int(cfg.TelegramDebounceWindow().Milliseconds()))
			fmt.Printf("  telegram.defaults.render_format: %q\n", cfg.TelegramRenderFormat())
			fmt.Printf("  session.timeout_min: %d\n", int(cfg.SessionTimeout().Minutes()))
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
					fmt.Printf("    - internal_chat_id=%s provider=%s provider_chat_id=%q enabled=%t process_tools=%t gpus=%q title=%q\n", chat.ID.String(), chat.ProviderType, chat.ProviderChatID, chat.Enabled, cfg.ChatProcessToolsEnabledByID(chat.ID), cfg.ChatGPUsByID(chat.ID), title)
					workspacePath := cfg.ChatWorkspaceHostPathByID(chat.ID)
					if workspacePath == "" {
						workspacePath = "<global/default>"
					}
					fmt.Printf("      workspace_host_path: %s\n", workspacePath)
					skills := cfg.ChatSkillsByID(chat.ID)
					if len(skills) == 0 {
						fmt.Println("      skills: <none>")
					} else {
						fmt.Printf("      skills: %v\n", skills)
					}
					commands := cfg.ChatHostbridgeAllowedCommandsByID(chat.ID)
					if len(commands) == 0 {
						fmt.Println("      hostbridge.allowed_commands: <defaults only>")
						continue
					}
					names := hostbridge.AllowedCommandNames(commands)
					fmt.Printf("      hostbridge.allowed_commands: names=%v\n", names)
					for _, name := range names {
						spec := commands[name]
						fmt.Printf("        %s => name=%q args=%v dir=%q allow_extra_args=%t delay=%q\n", name, spec.Name, spec.Args, spec.Dir, spec.AllowExtraArgs, spec.Delay)
					}
				}
			}
			fmt.Println("  help:")
			fmt.Println("    set a root value with: ctgbot config --set-docker-image <image>")
			fmt.Println("    enable a chat with: ctgbot config chat <chat-id> --set-enabled true")
			fmt.Println("    set chat process tools with: ctgbot config chat <chat-id> --set-process-tools-enabled true")
			fmt.Println("    set chat GPUs with: ctgbot config chat <chat-id> --set-gpus all")
			fmt.Println("    set a chat workspace with: ctgbot config chat <chat-id> --set-workspace-host-path <path>")
			fmt.Println("    set a chat hostbridge command with: ctgbot config chat <chat-id> hostbridge <alias> --set-command <command>")
			fmt.Println("    set a chat hostbridge dir with: ctgbot config chat <chat-id> hostbridge <alias> --set-dir <dir>")
			fmt.Println("    set a chat hostbridge args with: ctgbot config chat <chat-id> hostbridge <alias> --set-args arg1,arg2")
			fmt.Println("    set a chat hostbridge delay with: ctgbot config chat <chat-id> hostbridge <alias> --set-delay <ms|duration>")
			fmt.Println("    remove a chat hostbridge alias with: ctgbot config chat <chat-id> hostbridge <alias> --remove true")
			fmt.Println("    add a chat skill with: ctgbot config chat <chat-id> --add-skill <absolute-skill-dir>")
			fmt.Println("    remove a chat skill with: ctgbot config chat <chat-id> --remove-skill <absolute-skill-dir>")
			return nil
		})

		b.Route("config", func(b *clir.Builder) {
			if initErr != nil {
				return
			}
			if err := setter.RegisterSubroutes(b); err != nil {
				panic(err)
			}
		})
	})
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
