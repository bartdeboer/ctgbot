package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
	"github.com/bartdeboer/go-codextgbot/internal/botengine"
)

func registerConfigRoutes(r *clir.Router, store *clistate.Store, globalStore *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("config", "Show or update codextgbot config", func(req *clir.Request) error {
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
			setWorkspaceHostPath := fs.String("set-workspace-host-path", "", "Persist docker.workspace_host_path into config")
			setHostbridgeSocketPath := fs.String("set-hostbridge-socket-path", "", "Persist docker.hostbridge_socket_path into config")
			setCodexModel := fs.String("set-codex-model", "", "Persist codex.model into config")
			setCodexSharedHomePath := fs.String("set-codex-shared-home-path", "", "Persist codex.shared_home_host_path into config")
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
				*setWorkspaceHostPath == "" &&
				*setHostbridgeSocketPath == "" &&
				*setCodexModel == "" &&
				*setCodexSharedHomePath == "" &&
				*setSessionTimeoutMin == 0 &&
				*setPollTimeoutSec == 0 &&
				*enableChatID == 0 &&
				*disableChatID == 0 &&
				!*writeFullAuto {
				cfg, err := botengine.NewConfig("", store)
				if err != nil {
					return err
				}

				fmt.Println("Current config:")
				fmt.Printf("  project_dir: %q\n", globalStore.GetString("project_dir", ""))
				fmt.Printf("  telegram.token: %q\n", store.GetString("telegram.token", ""))
				fmt.Printf("  docker.image: %q\n", cfg.DockerImage())
				fmt.Printf("  docker.workspace_host_path: %q\n", cfg.DefaultWorkspaceHostPath())
				fmt.Printf("  docker.hostbridge_socket_path: %q\n", cfg.HostbridgeSocketPath())
				fmt.Printf("  codex.model: %q\n", cfg.CodexModel())
				fmt.Printf("  codex.shared_home_host_path: %q\n", cfg.SharedCodexRoot())
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
					}
				}
				fmt.Println("  help:")
				fmt.Println("    enable a chat with: codextgbot config --enable-chat-id <chat-id>")
				fmt.Println("    disable a chat with: codextgbot config --disable-chat-id <chat-id>")
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
			if *setWorkspaceHostPath != "" {
				if err := store.PersistString("docker.workspace_host_path", *setWorkspaceHostPath); err != nil {
					return fmt.Errorf("persist docker.workspace_host_path: %w", err)
				}
			}
			if *setHostbridgeSocketPath != "" {
				if err := store.PersistString("docker.hostbridge_socket_path", *setHostbridgeSocketPath); err != nil {
					return fmt.Errorf("persist docker.hostbridge_socket_path: %w", err)
				}
			}
			if *setCodexModel != "" {
				if err := store.PersistString("codex.model", *setCodexModel); err != nil {
					return fmt.Errorf("persist codex.model: %w", err)
				}
			}
			if *setCodexSharedHomePath != "" {
				if err := store.PersistString("codex.shared_home_host_path", *setCodexSharedHomePath); err != nil {
					return fmt.Errorf("persist codex.shared_home_host_path: %w", err)
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
				cfg, err := botengine.NewConfig("", store)
				if err != nil {
					return err
				}
				if err := cfg.SetChatEnabled(*enableChatID, true); err != nil {
					return fmt.Errorf("enable chat %d: %w", *enableChatID, err)
				}
			}
			if *disableChatID != 0 {
				cfg, err := botengine.NewConfig("", store)
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
			if len(updates) == 0 {
				fmt.Println("config updated")
			} else {
				fmt.Printf("config updated: %s\n", strings.Join(updates, ", "))
			}
			return nil
		})
	})
}
