package main

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
	"github.com/bartdeboer/go-ctgbot/internal/appconfig"
	"github.com/bartdeboer/go-ctgbot/internal/hostbridge"
)

func registerConfigRoutes(r *clir.Router, store *clistate.Store, globalStore *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("config", "Show or update ctgbot config", func(req *clir.Request) error {
			if store == nil {
				return fmt.Errorf("no cwd config store available")
			}
			if globalStore == nil {
				return fmt.Errorf("no global config store available")
			}

			fs := flag.NewFlagSet("config", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)

			setTelegramToken := fs.String("set-telegram-token", "", "Persist telegram.token into config")
			setDockerImage := fs.String("set-docker-image", "", "Persist docker.image into config")
			setDockerCLIContainerName := fs.String("set-docker-cli-container-name", "", "Persist docker.cli_container_name into config")
			setWorkspaceHostPath := fs.String("set-workspace-host-path", "", "Persist docker.workspace_host_path into config")
			setHostbridgeTCPListenAddr := fs.String("set-hostbridge-tcp-listen-addr", "", "Persist hostbridge.tcp_listen_addr into config")
			setContainerHostbridgeTCPAddr := fs.String("set-container-hostbridge-tcp-addr", "", "Persist docker.container_hostbridge_tcp_addr into config")
			var allowChatHostbridgeCommand chatCommandFlag
			var removeChatHostbridgeCommand chatCommandFlag
			fs.Var(&allowChatHostbridgeCommand, "allow-chat-hostbridge-command", "Persist a chat-local hostbridge command in the form <chat-id>:<command-or-absolute-path>")
			fs.Var(&removeChatHostbridgeCommand, "remove-chat-hostbridge-command", "Remove a chat-local hostbridge command by basename in the form <chat-id>:<name>")
			setCodexModel := fs.String("set-codex-model", "", "Persist codex.model into config")
			setCodexCLIHomePath := fs.String("set-codex-cli-home-path", "", "Persist codex.cli_home_host_path into config")
			setCodexSharedHomePath := fs.String("set-codex-shared-home-path", "", "Deprecated alias for --set-codex-cli-home-path")
			setSessionTimeoutMin := fs.Int("set-session-timeout-min", 0, "Persist session.timeout_min into config")
			setPollTimeoutSec := fs.Int("set-poll-timeout-sec", 0, "Persist telegram.defaults.poll_timeout_sec into config")
			setFullAuto := fs.Bool("set-codex-full-auto", true, "Persist codex.full_auto into config")
			writeFullAuto := fs.Bool("write-codex-full-auto", false, "Write the --set-codex-full-auto value into config")
			enableChatID := fs.Int64("enable-chat-id", 0, "Enable a Telegram chat by id")
			disableChatID := fs.Int64("disable-chat-id", 0, "Disable a Telegram chat by id")

			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			if *setTelegramToken == "" &&
				*setDockerImage == "" &&
				*setDockerCLIContainerName == "" &&
				*setWorkspaceHostPath == "" &&
				*setHostbridgeTCPListenAddr == "" &&
				*setContainerHostbridgeTCPAddr == "" &&
				len(allowChatHostbridgeCommand.values) == 0 &&
				len(removeChatHostbridgeCommand.values) == 0 &&
				*setCodexModel == "" &&
				*setCodexCLIHomePath == "" &&
				*setCodexSharedHomePath == "" &&
				*setSessionTimeoutMin == 0 &&
				*setPollTimeoutSec == 0 &&
				*enableChatID == 0 &&
				*disableChatID == 0 &&
				!*writeFullAuto {
				cfg, err := appconfig.NewConfig("", store)
				if err != nil {
					return err
				}

				fmt.Println("Current config:")
				fmt.Printf("  project_dir: %q\n", globalStore.GetString("project_dir", ""))
				fmt.Printf("  telegram.token: %q\n", store.GetString("telegram.token", ""))
				fmt.Printf("  docker.image: %q\n", cfg.DockerImage())
				fmt.Printf("  docker.cli_container_name: %q\n", cfg.DockerCLIContainerName())
				fmt.Printf("  docker.workspace_host_path: %q\n", cfg.DefaultWorkspaceHostPath())
				fmt.Printf("  hostbridge.tcp_listen_addr: %q\n", cfg.HostbridgeTCPListenAddr())
				fmt.Printf("  docker.container_hostbridge_tcp_addr: %q\n", cfg.ContainerHostbridgeTCPAddr())
				fmt.Printf("  codex.model: %q\n", cfg.CodexModel())
				fmt.Printf("  codex.cli_home_host_path: %q\n", cfg.CodexCLIHomeRoot())
				fmt.Printf("  codex.login_callback_port: %d (fixed)\n", appconfig.CodexLoginCallbackPort)
				fmt.Printf("  codex.full_auto: %t\n", cfg.CodexFullAuto())
				fmt.Printf("  telegram.defaults.poll_timeout_sec: %d\n", int(cfg.PollTimeout().Seconds()))
				fmt.Printf("  session.timeout_min: %d\n", int(cfg.SessionTimeout().Minutes()))
				fmt.Println("  known_chats:")
				chats := cfg.KnownChats()
				if len(chats) == 0 {
					fmt.Println("    <none yet>")
				} else {
					for _, chat := range chats {
						title := chat.ChatTitle
						if title == "" {
							title = "<untitled>"
						}
						fmt.Printf("    - id=%d scope=%s enabled=%t title=%q\n", chat.ChatID, chat.Scope, chat.Enabled, title)
						specs := cfg.ChatHostbridgeAllowedCommandSpecs(chat.ChatID)
						if len(specs) == 0 {
							fmt.Println("      hostbridge.allowed_commands: <defaults only>")
							continue
						}
						names := hostbridge.AllowedCommandNames(hostbridge.AllowedCommandsFromSpecs(specs))
						fmt.Printf("      hostbridge.allowed_commands: names=%v specs=%v\n", names, specs)
					}
				}
				fmt.Println("  help:")
				fmt.Println("    enable a chat with: ctgbot config --enable-chat-id <chat-id>")
				fmt.Println("    disable a chat with: ctgbot config --disable-chat-id <chat-id>")
				fmt.Println("    allow a chat hostbridge command with: ctgbot config --allow-chat-hostbridge-command <chat-id>:<command-or-absolute-path>")
				return nil
			}

			if *setTelegramToken != "" {
				if err := store.PersistString("telegram.token", *setTelegramToken); err != nil {
					return fmt.Errorf("persist telegram.token: %w", err)
				}
			}
			if *setDockerImage != "" {
				if err := store.PersistString("docker.image", *setDockerImage); err != nil {
					return fmt.Errorf("persist docker.image: %w", err)
				}
			}
			if *setDockerCLIContainerName != "" {
				if err := store.PersistString("docker.cli_container_name", *setDockerCLIContainerName); err != nil {
					return fmt.Errorf("persist docker.cli_container_name: %w", err)
				}
			}
			if *setWorkspaceHostPath != "" {
				if err := store.PersistString("docker.workspace_host_path", *setWorkspaceHostPath); err != nil {
					return fmt.Errorf("persist docker.workspace_host_path: %w", err)
				}
			}
			if *setHostbridgeTCPListenAddr != "" {
				if err := store.PersistString("hostbridge.tcp_listen_addr", *setHostbridgeTCPListenAddr); err != nil {
					return fmt.Errorf("persist hostbridge.tcp_listen_addr: %w", err)
				}
			}
			if *setContainerHostbridgeTCPAddr != "" {
				if err := store.PersistString("docker.container_hostbridge_tcp_addr", *setContainerHostbridgeTCPAddr); err != nil {
					return fmt.Errorf("persist docker.container_hostbridge_tcp_addr: %w", err)
				}
			}
			if len(allowChatHostbridgeCommand.values) > 0 || len(removeChatHostbridgeCommand.values) > 0 {
				cfg, err := appconfig.NewConfig("", store)
				if err != nil {
					return err
				}
				for chatID, specs := range allowChatHostbridgeCommand.values {
					for _, spec := range specs {
						if err := cfg.SetChatHostbridgeAllowedCommand(chatID, spec); err != nil {
							return fmt.Errorf("persist hostbridge allowed command for chat %d (%q): %w", chatID, spec, err)
						}
					}
				}
				for chatID, names := range removeChatHostbridgeCommand.values {
					for _, name := range names {
						if err := cfg.RemoveChatHostbridgeAllowedCommand(chatID, name); err != nil {
							return fmt.Errorf("remove hostbridge allowed command for chat %d (%q): %w", chatID, name, err)
						}
					}
				}
			}
			if *setCodexModel != "" {
				if err := store.PersistString("codex.model", *setCodexModel); err != nil {
					return fmt.Errorf("persist codex.model: %w", err)
				}
			}
			codexHomeOverride := *setCodexCLIHomePath
			if codexHomeOverride == "" {
				codexHomeOverride = *setCodexSharedHomePath
			}
			if codexHomeOverride != "" {
				if err := store.PersistString("codex.cli_home_host_path", codexHomeOverride); err != nil {
					return fmt.Errorf("persist codex.cli_home_host_path: %w", err)
				}
			}
			if *setSessionTimeoutMin != 0 {
				if err := store.PersistInt("session.timeout_min", *setSessionTimeoutMin); err != nil {
					return fmt.Errorf("persist session.timeout_min: %w", err)
				}
			}
			if *setPollTimeoutSec != 0 {
				if err := store.PersistInt("telegram.defaults.poll_timeout_sec", *setPollTimeoutSec); err != nil {
					return fmt.Errorf("persist telegram.defaults.poll_timeout_sec: %w", err)
				}
			}
			if *writeFullAuto {
				if err := store.PersistBool("codex.full_auto", *setFullAuto); err != nil {
					return fmt.Errorf("persist codex.full_auto: %w", err)
				}
			}
			if *enableChatID != 0 {
				cfg, err := appconfig.NewConfig("", store)
				if err != nil {
					return err
				}
				if err := cfg.SetChatEnabled(*enableChatID, true); err != nil {
					return fmt.Errorf("enable chat %d: %w", *enableChatID, err)
				}
			}
			if *disableChatID != 0 {
				cfg, err := appconfig.NewConfig("", store)
				if err != nil {
					return err
				}
				if err := cfg.SetChatEnabled(*disableChatID, false); err != nil {
					return fmt.Errorf("disable chat %d: %w", *disableChatID, err)
				}
			}

			var updates []string
			if *enableChatID != 0 {
				updates = append(updates, fmt.Sprintf("enabled chat %d", *enableChatID))
			}
			if *disableChatID != 0 {
				updates = append(updates, fmt.Sprintf("disabled chat %d", *disableChatID))
			}
			for chatID, specs := range allowChatHostbridgeCommand.values {
				for _, spec := range specs {
					updates = append(updates, fmt.Sprintf("allowed chat %d hostbridge command %s", chatID, spec))
				}
			}
			for chatID, names := range removeChatHostbridgeCommand.values {
				for _, name := range names {
					updates = append(updates, fmt.Sprintf("removed chat %d hostbridge command %s", chatID, name))
				}
			}
			if len(updates) == 0 {
				fmt.Println("config updated")
			} else {
				fmt.Printf("config updated: %s\n", strings.Join(updates, ", "))
			}
			return nil
		})
	})
}

type chatCommandFlag struct {
	values map[int64][]string
}

func (f *chatCommandFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	var parts []string
	for chatID, values := range f.values {
		for _, value := range values {
			parts = append(parts, fmt.Sprintf("%d:%s", chatID, value))
		}
	}
	return strings.Join(parts, ",")
}

func (f *chatCommandFlag) Set(v string) error {
	if f.values == nil {
		f.values = map[int64][]string{}
	}
	chatRaw, value, ok := strings.Cut(v, ":")
	if !ok {
		return fmt.Errorf("expected <chat-id>:<command-or-absolute-path>")
	}
	chatRaw = strings.TrimSpace(chatRaw)
	value = strings.TrimSpace(value)
	if chatRaw == "" || value == "" {
		return fmt.Errorf("expected <chat-id>:<command-or-absolute-path>")
	}
	chatID, err := strconv.ParseInt(chatRaw, 10, 64)
	if err != nil {
		return fmt.Errorf("invalid chat id %q", chatRaw)
	}
	f.values[chatID] = append(f.values[chatID], value)
	return nil
}
