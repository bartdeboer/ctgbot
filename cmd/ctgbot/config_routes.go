package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
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
			setBuildCompilerPath := fs.String("set-build-compiler-path", "", "Persist build.compiler_path into global config")
			setDockerImage := fs.String("set-docker-image", "", "Persist docker.image into config")
			setDockerCLIContainerName := fs.String("set-docker-cli-container-name", "", "Persist docker.cli_container_name into config")
			setWorkspaceHostPath := fs.String("set-workspace-host-path", "", "Persist docker.workspace_host_path into config")
			var setChatWorkspaceHostPath chatValueFlag
			setHostbridgeTCPListenAddr := fs.String("set-hostbridge-tcp-listen-addr", "", "Persist hostbridge.tcp_listen_addr into config")
			setContainerHostbridgeTCPAddr := fs.String("set-container-hostbridge-tcp-addr", "", "Persist docker.container_hostbridge_tcp_addr into config")
			var allowChatHostbridgeCommand chatAllowedCommandFlag
			var setChatHostbridgeCommandDir chatAliasValueFlag
			var setChatHostbridgeCommandArgs chatAliasArgsFlag
			var setChatHostbridgeCommandAllowExtraArgs chatAliasBoolFlag
			var removeChatHostbridgeCommand chatAliasNameFlag
			var addChatSkill uuidValueFlag
			var removeChatSkill uuidValueFlag
			fs.Var(&setChatWorkspaceHostPath, "set-chat-workspace-host-path", "Persist a Telegram chat-local workspace host path in the form <provider-chat-id>:<path>")
			fs.Var(&allowChatHostbridgeCommand, "allow-chat-hostbridge-command", "Persist a Telegram chat-local hostbridge command in the form <provider-chat-id>:<alias>=<command> (or legacy <provider-chat-id>:<command>)")
			fs.Var(&setChatHostbridgeCommandDir, "set-chat-hostbridge-command-dir", "Persist a Telegram chat-local hostbridge command dir in the form <provider-chat-id>:<alias>=<dir>")
			fs.Var(&setChatHostbridgeCommandArgs, "set-chat-hostbridge-command-args", "Persist a Telegram chat-local hostbridge command args in the form <provider-chat-id>:<alias>=arg1,arg2")
			fs.Var(&setChatHostbridgeCommandAllowExtraArgs, "set-chat-hostbridge-command-allow-extra-args", "Persist whether a Telegram chat-local hostbridge command allows extra args in the form <provider-chat-id>:<alias>=true")
			fs.Var(&removeChatHostbridgeCommand, "remove-chat-hostbridge-command", "Remove a Telegram chat-local hostbridge command in the form <provider-chat-id>:<alias>")
			fs.Var(&addChatSkill, "add-chat-skill", "Persist a chat-local skill in the form <internal-chat-id>:<absolute-skill-dir>")
			fs.Var(&removeChatSkill, "remove-chat-skill", "Remove a chat-local skill in the form <internal-chat-id>:<absolute-skill-dir>")
			setCodexModel := fs.String("set-codex-model", "", "Persist codex.model into config")
			setCodexCLIHomePath := fs.String("set-codex-cli-home-path", "", "Persist codex.cli_home_host_path into config")
			setCodexSharedHomePath := fs.String("set-codex-shared-home-path", "", "Deprecated alias for --set-codex-cli-home-path")
			setSessionTimeoutMin := fs.Int("set-session-timeout-min", 0, "Persist session.timeout_min into config")
			setPollTimeoutSec := fs.Int("set-poll-timeout-sec", 0, "Persist telegram.defaults.poll_timeout_sec into config")
			setFullAuto := fs.Bool("set-codex-full-auto", true, "Persist codex.full_auto into config")
			writeFullAuto := fs.Bool("write-codex-full-auto", false, "Write the --set-codex-full-auto value into config")
			enableChatID := fs.Int64("enable-chat-id", 0, "Enable a Telegram chat by provider chat id")
			disableChatID := fs.Int64("disable-chat-id", 0, "Disable a Telegram chat by provider chat id")
			enableChatProcessToolsID := fs.Int64("enable-chat-process-tools", 0, "Enable Telegram chat process tools by provider chat id")
			disableChatProcessToolsID := fs.Int64("disable-chat-process-tools", 0, "Disable Telegram chat process tools by provider chat id")

			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			if *setTelegramToken == "" &&
				*setBuildCompilerPath == "" &&
				*setDockerImage == "" &&
				*setDockerCLIContainerName == "" &&
				*setWorkspaceHostPath == "" &&
				len(setChatWorkspaceHostPath.values) == 0 &&
				*setHostbridgeTCPListenAddr == "" &&
				*setContainerHostbridgeTCPAddr == "" &&
				len(allowChatHostbridgeCommand.values) == 0 &&
				len(setChatHostbridgeCommandDir.values) == 0 &&
				len(setChatHostbridgeCommandArgs.values) == 0 &&
				len(setChatHostbridgeCommandAllowExtraArgs.values) == 0 &&
				len(removeChatHostbridgeCommand.values) == 0 &&
				len(addChatSkill.values) == 0 &&
				len(removeChatSkill.values) == 0 &&
				*setCodexModel == "" &&
				*setCodexCLIHomePath == "" &&
				*setCodexSharedHomePath == "" &&
				*setSessionTimeoutMin == 0 &&
				*setPollTimeoutSec == 0 &&
				*enableChatID == 0 &&
				*disableChatID == 0 &&
				*enableChatProcessToolsID == 0 &&
				*disableChatProcessToolsID == 0 &&
				!*writeFullAuto {
				cfg, err := appconfig.NewConfig("", store)
				if err != nil {
					return err
				}
				if err := cfg.EnsurePaths(); err != nil {
					return err
				}

				fmt.Println("Current config:")
				fmt.Printf("  project_dir: %q\n", globalStore.GetString("project_dir", ""))
				fmt.Printf("  build.compiler_path: %q\n", globalStore.GetString("build.compiler_path", ""))
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
						title := chat.ProviderChatTitle
						if title == "" {
							title = "<untitled>"
						}
						fmt.Printf("    - internal_chat_id=%s provider=%s provider_chat_id=%q enabled=%t process_tools=%t title=%q\n", chat.ID.String(), chat.ProviderType, chat.ProviderChatID, chat.Enabled, cfg.ChatProcessToolsEnabledByID(chat.ID), title)
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
							fmt.Printf("        %s => name=%q args=%v dir=%q allow_extra_args=%t\n", name, spec.Name, spec.Args, spec.Dir, spec.AllowExtraArgs)
						}
					}
				}
				fmt.Println("  help:")
				fmt.Println("    compatibility note: mutation flags below still use Telegram provider chat ids, not internal chat UUIDs")
				fmt.Println("    enable a chat with: ctgbot config --enable-chat-id <provider-chat-id>")
				fmt.Println("    disable a chat with: ctgbot config --disable-chat-id <provider-chat-id>")
				fmt.Println("    enable chat process tools with: ctgbot config --enable-chat-process-tools <provider-chat-id>")
				fmt.Println("    disable chat process tools with: ctgbot config --disable-chat-process-tools <provider-chat-id>")
				fmt.Println("    set a chat workspace with: ctgbot config --set-chat-workspace-host-path <provider-chat-id>:<path>")
				fmt.Println("    allow a chat hostbridge command with: ctgbot config --allow-chat-hostbridge-command <provider-chat-id>:<alias>=<command>")
				fmt.Println("    set a chat hostbridge command dir with: ctgbot config --set-chat-hostbridge-command-dir <provider-chat-id>:<alias>=<dir>")
				fmt.Println("    set a chat hostbridge command args with: ctgbot config --set-chat-hostbridge-command-args <provider-chat-id>:<alias>=arg1,arg2")
				fmt.Println("    add a chat skill with: ctgbot config --add-chat-skill <internal-chat-id>:<absolute-skill-dir>")
				return nil
			}

			if *setTelegramToken != "" {
				if err := store.PersistString("telegram.token", *setTelegramToken); err != nil {
					return fmt.Errorf("persist telegram.token: %w", err)
				}
			}
			if *setBuildCompilerPath != "" {
				if err := globalStore.PersistString("build.compiler_path", *setBuildCompilerPath); err != nil {
					return fmt.Errorf("persist build.compiler_path: %w", err)
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
			if len(setChatWorkspaceHostPath.values) > 0 {
				cfg, err := appconfig.NewConfig("", store)
				if err != nil {
					return err
				}
				for chatID, specs := range setChatWorkspaceHostPath.values {
					for _, spec := range specs {
						if err := cfg.SetChatWorkspaceHostPath(chatID, spec); err != nil {
							return fmt.Errorf("persist workspace host path for telegram chat %d (%q): %w", chatID, spec, err)
						}
					}
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
			if len(allowChatHostbridgeCommand.values) > 0 ||
				len(setChatHostbridgeCommandDir.values) > 0 ||
				len(setChatHostbridgeCommandArgs.values) > 0 ||
				len(setChatHostbridgeCommandAllowExtraArgs.values) > 0 ||
				len(removeChatHostbridgeCommand.values) > 0 {
				cfg, err := appconfig.NewConfig("", store)
				if err != nil {
					return err
				}
				updates := collectChatHostbridgeCommandUpdates(
					cfg,
					allowChatHostbridgeCommand.values,
					setChatHostbridgeCommandDir.values,
					setChatHostbridgeCommandArgs.values,
					setChatHostbridgeCommandAllowExtraArgs.values,
				)
				for _, update := range updates {
					if err := cfg.SetChatHostbridgeAllowedCommand(update.chatID, update.alias, update.command); err != nil {
						return fmt.Errorf("persist hostbridge allowed command for telegram chat %d alias %q: %w", update.chatID, update.alias, err)
					}
				}
				for chatID, names := range removeChatHostbridgeCommand.values {
					for _, name := range names {
						if err := cfg.RemoveChatHostbridgeAllowedCommand(chatID, name); err != nil {
							return fmt.Errorf("remove hostbridge allowed command for telegram chat %d (%q): %w", chatID, name, err)
						}
					}
				}
			}
			if len(addChatSkill.values) > 0 || len(removeChatSkill.values) > 0 {
				cfg, err := appconfig.NewConfig("", store)
				if err != nil {
					return err
				}
				for chatID, skills := range addChatSkill.values {
					for _, skill := range skills {
						if err := cfg.AddChatSkillByID(chatID, skill); err != nil {
							return fmt.Errorf("add skill for chat %s (%q): %w", chatID.String(), skill, err)
						}
					}
				}
				for chatID, skills := range removeChatSkill.values {
					for _, skill := range skills {
						if err := cfg.RemoveChatSkillByID(chatID, skill); err != nil {
							return fmt.Errorf("remove skill for chat %s (%q): %w", chatID.String(), skill, err)
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
					return fmt.Errorf("enable telegram chat %d: %w", *enableChatID, err)
				}
			}
			if *disableChatID != 0 {
				cfg, err := appconfig.NewConfig("", store)
				if err != nil {
					return err
				}
				if err := cfg.SetChatEnabled(*disableChatID, false); err != nil {
					return fmt.Errorf("disable telegram chat %d: %w", *disableChatID, err)
				}
			}
			if *enableChatProcessToolsID != 0 {
				cfg, err := appconfig.NewConfig("", store)
				if err != nil {
					return err
				}
				if err := cfg.SetChatProcessToolsEnabled(*enableChatProcessToolsID, true); err != nil {
					return fmt.Errorf("enable telegram chat %d process tools: %w", *enableChatProcessToolsID, err)
				}
			}
			if *disableChatProcessToolsID != 0 {
				cfg, err := appconfig.NewConfig("", store)
				if err != nil {
					return err
				}
				if err := cfg.SetChatProcessToolsEnabled(*disableChatProcessToolsID, false); err != nil {
					return fmt.Errorf("disable telegram chat %d process tools: %w", *disableChatProcessToolsID, err)
				}
			}

			var updates []string
			if *enableChatID != 0 {
				updates = append(updates, fmt.Sprintf("enabled telegram chat %d", *enableChatID))
			}
			if *disableChatID != 0 {
				updates = append(updates, fmt.Sprintf("disabled telegram chat %d", *disableChatID))
			}
			if *enableChatProcessToolsID != 0 {
				updates = append(updates, fmt.Sprintf("enabled telegram chat %d process tools", *enableChatProcessToolsID))
			}
			if *disableChatProcessToolsID != 0 {
				updates = append(updates, fmt.Sprintf("disabled telegram chat %d process tools", *disableChatProcessToolsID))
			}
			for chatID, paths := range setChatWorkspaceHostPath.values {
				for _, path := range paths {
					updates = append(updates, fmt.Sprintf("set telegram chat %d workspace %s", chatID, path))
				}
			}
			for _, update := range collectChatHostbridgeCommandUpdates(nil, allowChatHostbridgeCommand.values, setChatHostbridgeCommandDir.values, setChatHostbridgeCommandArgs.values, setChatHostbridgeCommandAllowExtraArgs.values) {
				updates = append(updates, fmt.Sprintf("set telegram chat %d hostbridge command %s => name=%s args=%v dir=%s allow_extra_args=%t", update.chatID, update.alias, update.command.Name, update.command.Args, update.command.Dir, update.command.AllowExtraArgs))
			}
			for chatID, names := range removeChatHostbridgeCommand.values {
				for _, name := range names {
					updates = append(updates, fmt.Sprintf("removed telegram chat %d hostbridge command %s", chatID, name))
				}
			}
			for chatID, skills := range addChatSkill.values {
				for _, skill := range skills {
					updates = append(updates, fmt.Sprintf("added chat %s skill %s", chatID.String(), skill))
				}
			}
			for chatID, skills := range removeChatSkill.values {
				for _, skill := range skills {
					updates = append(updates, fmt.Sprintf("removed chat %s skill %s", chatID.String(), skill))
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

type chatAllowedCommandFlag struct {
	values map[int64]map[string]hostbridge.AllowedCommand
}

func (f *chatAllowedCommandFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	var parts []string
	for chatID, aliases := range f.values {
		for alias, command := range aliases {
			parts = append(parts, fmt.Sprintf("%d:%s=%s", chatID, alias, command.Name))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (f *chatAllowedCommandFlag) Set(v string) error {
	chatID, raw, err := parseChatValue(v, "<provider-chat-id>:<alias>=<command>")
	if err != nil {
		return err
	}

	alias := ""
	commandName := ""
	if key, value, ok := strings.Cut(raw, "="); ok {
		alias = strings.TrimSpace(key)
		commandName = strings.TrimSpace(value)
	} else {
		commandName = strings.TrimSpace(raw)
		alias = strings.TrimSpace(filepath.Base(commandName))
	}
	if alias == "" || commandName == "" {
		return fmt.Errorf("expected <provider-chat-id>:<alias>=<command>")
	}

	if f.values == nil {
		f.values = map[int64]map[string]hostbridge.AllowedCommand{}
	}
	if f.values[chatID] == nil {
		f.values[chatID] = map[string]hostbridge.AllowedCommand{}
	}
	current := f.values[chatID][alias]
	current.Name = commandName
	f.values[chatID][alias] = current
	return nil
}

type chatAliasValueFlag struct {
	values map[int64]map[string]string
}

func (f *chatAliasValueFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	var parts []string
	for chatID, aliases := range f.values {
		for alias, value := range aliases {
			parts = append(parts, fmt.Sprintf("%d:%s=%s", chatID, alias, value))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (f *chatAliasValueFlag) Set(v string) error {
	chatID, alias, value, err := parseChatAliasAssignment(v)
	if err != nil {
		return err
	}
	if f.values == nil {
		f.values = map[int64]map[string]string{}
	}
	if f.values[chatID] == nil {
		f.values[chatID] = map[string]string{}
	}
	f.values[chatID][alias] = value
	return nil
}

type chatAliasBoolFlag struct {
	values map[int64]map[string]bool
}

func (f *chatAliasBoolFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	var parts []string
	for chatID, aliases := range f.values {
		for alias, value := range aliases {
			parts = append(parts, fmt.Sprintf("%d:%s=%t", chatID, alias, value))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (f *chatAliasBoolFlag) Set(v string) error {
	chatID, alias, raw, err := parseChatAliasAssignment(v)
	if err != nil {
		return err
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		return fmt.Errorf("invalid bool %q", raw)
	}
	if f.values == nil {
		f.values = map[int64]map[string]bool{}
	}
	if f.values[chatID] == nil {
		f.values[chatID] = map[string]bool{}
	}
	f.values[chatID][alias] = parsed
	return nil
}

type chatAliasArgsFlag struct {
	values map[int64]map[string][]string
}

func (f *chatAliasArgsFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	var parts []string
	for chatID, aliases := range f.values {
		for alias, value := range aliases {
			parts = append(parts, fmt.Sprintf("%d:%s=%s", chatID, alias, strings.Join(value, ",")))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (f *chatAliasArgsFlag) Set(v string) error {
	chatID, alias, raw, err := parseChatAliasAssignment(v)
	if err != nil {
		return err
	}
	args, err := parseArgsCSV(raw)
	if err != nil {
		return err
	}
	if f.values == nil {
		f.values = map[int64]map[string][]string{}
	}
	if f.values[chatID] == nil {
		f.values[chatID] = map[string][]string{}
	}
	f.values[chatID][alias] = args
	return nil
}

type chatAliasNameFlag struct {
	values map[int64][]string
}

func (f *chatAliasNameFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	var parts []string
	for chatID, values := range f.values {
		for _, value := range values {
			parts = append(parts, fmt.Sprintf("%d:%s", chatID, value))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (f *chatAliasNameFlag) Set(v string) error {
	chatID, value, err := parseChatValue(v, "<provider-chat-id>:<alias>")
	if err != nil {
		return err
	}
	if f.values == nil {
		f.values = map[int64][]string{}
	}
	f.values[chatID] = append(f.values[chatID], strings.TrimSpace(value))
	return nil
}

type chatValueFlag struct {
	values map[int64][]string
}

func (f *chatValueFlag) String() string {
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

func (f *chatValueFlag) Set(v string) error {
	if f.values == nil {
		f.values = map[int64][]string{}
	}
	chatID, value, err := parseChatValue(v, "<provider-chat-id>:<path>")
	if err != nil {
		return err
	}
	f.values[chatID] = append(f.values[chatID], value)
	return nil
}

type uuidValueFlag struct {
	values map[modeluuid.UUID][]string
}

func (f *uuidValueFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	var parts []string
	for chatID, values := range f.values {
		for _, value := range values {
			parts = append(parts, fmt.Sprintf("%s:%s", chatID.String(), value))
		}
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
}

func (f *uuidValueFlag) Set(v string) error {
	if f.values == nil {
		f.values = map[modeluuid.UUID][]string{}
	}
	chatRaw, value, ok := strings.Cut(v, ":")
	if !ok {
		return fmt.Errorf("expected <internal-chat-id>:<value>")
	}
	chatRaw = strings.TrimSpace(chatRaw)
	value = strings.TrimSpace(value)
	if chatRaw == "" || value == "" {
		return fmt.Errorf("expected <internal-chat-id>:<value>")
	}
	chatID, err := modeluuid.Parse(chatRaw)
	if err != nil {
		return fmt.Errorf("invalid internal chat id %q", chatRaw)
	}
	f.values[chatID] = append(f.values[chatID], value)
	return nil
}

type chatHostbridgeCommandUpdate struct {
	chatID  int64
	alias   string
	command hostbridge.AllowedCommand
}

func collectChatHostbridgeCommandUpdates(cfg *appconfig.Config, allowed map[int64]map[string]hostbridge.AllowedCommand, dirs map[int64]map[string]string, args map[int64]map[string][]string, allowExtra map[int64]map[string]bool) []chatHostbridgeCommandUpdate {
	keys := map[[2]string]struct{}{}
	addKeys := func(chatID int64, aliases map[string]struct{}) {
		for alias := range aliases {
			keys[[2]string{strconv.FormatInt(chatID, 10), alias}] = struct{}{}
		}
	}
	for chatID, aliases := range allowed {
		seen := map[string]struct{}{}
		for alias := range aliases {
			seen[alias] = struct{}{}
		}
		addKeys(chatID, seen)
	}
	for chatID, aliases := range dirs {
		seen := map[string]struct{}{}
		for alias := range aliases {
			seen[alias] = struct{}{}
		}
		addKeys(chatID, seen)
	}
	for chatID, aliases := range args {
		seen := map[string]struct{}{}
		for alias := range aliases {
			seen[alias] = struct{}{}
		}
		addKeys(chatID, seen)
	}
	for chatID, aliases := range allowExtra {
		seen := map[string]struct{}{}
		for alias := range aliases {
			seen[alias] = struct{}{}
		}
		addKeys(chatID, seen)
	}

	pairs := make([][2]string, 0, len(keys))
	for pair := range keys {
		pairs = append(pairs, pair)
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i][0] != pairs[j][0] {
			return pairs[i][0] < pairs[j][0]
		}
		return pairs[i][1] < pairs[j][1]
	})

	updates := make([]chatHostbridgeCommandUpdate, 0, len(pairs))
	for _, pair := range pairs {
		chatID, _ := strconv.ParseInt(pair[0], 10, 64)
		alias := pair[1]
		command := hostbridge.AllowedCommand{}
		if cfg != nil {
			if existing := cfg.ChatHostbridgeAllowedCommands(chatID); len(existing) > 0 {
				command = existing[alias]
			}
		}
		if spec, ok := allowed[chatID][alias]; ok {
			command.Name = spec.Name
		}
		if value, ok := dirs[chatID][alias]; ok {
			command.Dir = value
		}
		if value, ok := args[chatID][alias]; ok {
			command.Args = append([]string{}, value...)
		}
		if value, ok := allowExtra[chatID][alias]; ok {
			command.AllowExtraArgs = value
		}
		updates = append(updates, chatHostbridgeCommandUpdate{
			chatID:  chatID,
			alias:   alias,
			command: command,
		})
	}
	return updates
}

func parseChatValue(v string, expected string) (int64, string, error) {
	chatRaw, value, ok := strings.Cut(v, ":")
	if !ok {
		return 0, "", fmt.Errorf("expected %s", expected)
	}
	chatRaw = strings.TrimSpace(chatRaw)
	value = strings.TrimSpace(value)
	if chatRaw == "" || value == "" {
		return 0, "", fmt.Errorf("expected %s", expected)
	}
	chatID, err := strconv.ParseInt(chatRaw, 10, 64)
	if err != nil {
		return 0, "", fmt.Errorf("invalid chat id %q", chatRaw)
	}
	return chatID, value, nil
}

func parseChatAliasAssignment(v string) (int64, string, string, error) {
	chatID, raw, err := parseChatValue(v, "<provider-chat-id>:<alias>=<value>")
	if err != nil {
		return 0, "", "", err
	}
	alias, value, ok := strings.Cut(raw, "=")
	if !ok {
		return 0, "", "", fmt.Errorf("expected <provider-chat-id>:<alias>=<value>")
	}
	alias = strings.TrimSpace(alias)
	value = strings.TrimSpace(value)
	if alias == "" || value == "" {
		return 0, "", "", fmt.Errorf("expected <provider-chat-id>:<alias>=<value>")
	}
	return chatID, alias, value, nil
}

func parseArgsCSV(raw string) ([]string, error) {
	reader := csv.NewReader(strings.NewReader(raw))
	values, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("parse args csv: %w", err)
	}
	args := make([]string, 0, len(values))
	for _, value := range values {
		args = append(args, strings.TrimSpace(value))
	}
	if len(args) == 1 && args[0] == "" {
		return nil, nil
	}
	return args, nil
}
