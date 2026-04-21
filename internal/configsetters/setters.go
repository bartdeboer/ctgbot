package configsetters

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/go-clistate"
)

type ConfigSetters struct {
	State  *appstate.Config
	Local  *clistate.Store
	Global *clistate.Store
}

type ChatRoute struct {
	ChatID string `arg:"chatID" segment:"chat"`
}

type ChatHostbridgeAliasRoute struct {
	ChatID string `arg:"chatID" segment:"chat"`
	Alias  string `arg:"alias" segment:"hostbridge"`
}

type SetTelegramTokenInput struct {
	SetTelegramToken string `flag:"set-telegram-token"`
}

type SetTelegramAdminUserIDInput struct {
	SetTelegramAdminUserID int64 `flag:"set-telegram-admin-user-id"`
}

type SetBuildCompilerPathInput struct {
	SetBuildCompilerPath string `flag:"set-build-compiler-path"`
}

type SetDockerImageInput struct {
	SetDockerImage string `flag:"set-docker-image"`
}

type SetDockerCLIContainerNameInput struct {
	SetDockerCLIContainerName string `flag:"set-docker-cli-container-name"`
}

type SetWorkspaceHostPathInput struct {
	SetWorkspaceHostPath string `flag:"set-workspace-host-path"`
}

type SetHostbridgeTCPListenAddrInput struct {
	SetHostbridgeTCPListenAddr string `flag:"set-hostbridge-tcp-listen-addr"`
}

type SetContainerHostbridgeTCPAddrInput struct {
	SetContainerHostbridgeTCPAddr string `flag:"set-container-hostbridge-tcp-addr"`
}

type SetCodexModelInput struct {
	SetCodexModel string `flag:"set-codex-model"`
}

type SetCodexProfileHostPathInput struct {
	SetCodexProfileHostPath string `flag:"set-codex-profile-host-path"`
}

type SetCodexCLIHomePathInput struct {
	SetCodexCLIHomePath string `flag:"set-codex-cli-home-path"`
}

type SetCodexSharedHomePathInput struct {
	SetCodexSharedHomePath string `flag:"set-codex-shared-home-path"`
}

type SetSessionTimeoutMinInput struct {
	SetSessionTimeoutMin string `flag:"set-session-timeout-min"`
}

type SetPollTimeoutSecInput struct {
	SetPollTimeoutSec string `flag:"set-poll-timeout-sec"`
}

type SetChatEnabledInput struct {
	ChatRoute
	SetEnabled bool `flag:"set-enabled"`
}

type SetChatProcessToolsEnabledInput struct {
	ChatRoute
	SetProcessToolsEnabled bool `flag:"set-process-tools-enabled"`
}

type SetChatInteractiveInterruptEnabledInput struct {
	ChatRoute
	SetInteractiveInterruptEnabled bool `flag:"set-interactive-interrupt-enabled"`
}

type SetChatGPUsInput struct {
	ChatRoute
	SetGPUs string `flag:"set-gpus"`
}

type SetChatCodexProfileHostPathInput struct {
	ChatRoute
	SetCodexProfileHostPath string `flag:"set-codex-profile-host-path"`
}

type SetChatWorkspaceHostPathInput struct {
	ChatRoute
	SetWorkspaceHostPath string `flag:"set-workspace-host-path"`
}

type SetChatHostbridgeAliasCommandInput struct {
	ChatHostbridgeAliasRoute
	SetCommand string `flag:"set-command"`
}

type SetChatHostbridgeAliasDirInput struct {
	ChatHostbridgeAliasRoute
	SetDir string `flag:"set-dir"`
}

type SetChatHostbridgeAliasArgsInput struct {
	ChatHostbridgeAliasRoute
	SetArgs string `flag:"set-args"`
}

type SetChatHostbridgeAliasAllowExtraArgsInput struct {
	ChatHostbridgeAliasRoute
	SetAllowExtraArgs bool `flag:"set-allow-extra-args"`
}

type SetChatHostbridgeAliasDelayInput struct {
	ChatHostbridgeAliasRoute
	SetDelay string `flag:"set-delay"`
}

type SetChatHostbridgeAliasRemovedInput struct {
	ChatHostbridgeAliasRoute
	Remove bool `flag:"remove"`
}

type SetChatSkillAddedInput struct {
	ChatRoute
	AddSkill string `flag:"add-skill"`
}

type SetChatSkillRemovedInput struct {
	ChatRoute
	RemoveSkill string `flag:"remove-skill"`
}

func NewConfigSetters(state *appstate.Config, local *clistate.Store, global *clistate.Store) *ConfigSetters {
	return &ConfigSetters{
		State:  state,
		Local:  local,
		Global: global,
	}
}

func (c *ConfigSetters) SetTelegramToken(in SetTelegramTokenInput) error {
	if c == nil || c.Local == nil {
		return fmt.Errorf("missing local config store")
	}
	return c.Local.PersistString("telegram.token", in.SetTelegramToken)
}

func (c *ConfigSetters) SetBuildCompilerPath(in SetBuildCompilerPathInput) error {
	if c == nil || c.Global == nil {
		return fmt.Errorf("missing global config store")
	}
	return c.Global.PersistString("build.compiler_path", in.SetBuildCompilerPath)
}

func (c *ConfigSetters) SetDockerImage(in SetDockerImageInput) error {
	if c == nil || c.Local == nil {
		return fmt.Errorf("missing local config store")
	}
	return c.Local.PersistString("docker.image", in.SetDockerImage)
}

func (c *ConfigSetters) SetDockerCLIContainerName(in SetDockerCLIContainerNameInput) error {
	if c == nil || c.Local == nil {
		return fmt.Errorf("missing local config store")
	}
	return c.Local.PersistString("docker.cli_container_name", in.SetDockerCLIContainerName)
}

func (c *ConfigSetters) SetWorkspaceHostPath(in SetWorkspaceHostPathInput) error {
	if c == nil || c.Local == nil {
		return fmt.Errorf("missing local config store")
	}
	value := strings.TrimSpace(in.SetWorkspaceHostPath)
	if c.State != nil {
		resolved, err := c.State.ResolveWorkspaceHostPath(value)
		if err != nil {
			return err
		}
		value = resolved
	}
	return c.Local.PersistString("docker.workspace_host_path", value)
}

func (c *ConfigSetters) SetHostbridgeTCPListenAddr(in SetHostbridgeTCPListenAddrInput) error {
	if c == nil || c.Local == nil {
		return fmt.Errorf("missing local config store")
	}
	return c.Local.PersistString("hostbridge.tcp_listen_addr", in.SetHostbridgeTCPListenAddr)
}

func (c *ConfigSetters) SetContainerHostbridgeTCPAddr(in SetContainerHostbridgeTCPAddrInput) error {
	if c == nil || c.Local == nil {
		return fmt.Errorf("missing local config store")
	}
	return c.Local.PersistString("docker.container_hostbridge_tcp_addr", in.SetContainerHostbridgeTCPAddr)
}

func (c *ConfigSetters) SetCodexModel(in SetCodexModelInput) error {
	if c == nil || c.Local == nil {
		return fmt.Errorf("missing local config store")
	}
	return c.Local.PersistString("codex.model", in.SetCodexModel)
}

func (c *ConfigSetters) SetCodexProfileHostPath(in SetCodexProfileHostPathInput) error {
	if c == nil || c.Local == nil {
		return fmt.Errorf("missing local config store")
	}
	value := strings.TrimSpace(in.SetCodexProfileHostPath)
	if value == "" {
		return c.Local.PersistString("codex.profile_host_path", "")
	}
	abs, err := filepath.Abs(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return err
	}
	return c.Local.PersistString("codex.profile_host_path", abs)
}

func (c *ConfigSetters) SetCodexCLIHomePath(in SetCodexCLIHomePathInput) error {
	return c.SetCodexProfileHostPath(SetCodexProfileHostPathInput{SetCodexProfileHostPath: in.SetCodexCLIHomePath})
}

func (c *ConfigSetters) SetCodexSharedHomePath(in SetCodexSharedHomePathInput) error {
	return c.SetCodexProfileHostPath(SetCodexProfileHostPathInput{SetCodexProfileHostPath: in.SetCodexSharedHomePath})
}

func (c *ConfigSetters) SetSessionTimeoutMin(in SetSessionTimeoutMinInput) error {
	if c == nil || c.Local == nil {
		return fmt.Errorf("missing local config store")
	}
	return c.Local.PersistString("session.timeout_min", strings.TrimSpace(in.SetSessionTimeoutMin))
}

func (c *ConfigSetters) SetPollTimeoutSec(in SetPollTimeoutSecInput) error {
	if c == nil || c.Local == nil {
		return fmt.Errorf("missing local config store")
	}
	return c.Local.PersistString("telegram.defaults.poll_timeout_sec", strings.TrimSpace(in.SetPollTimeoutSec))
}

func (c *ConfigSetters) SetChatEnabled(in SetChatEnabledInput) error {
	chatID, err := parseChatID(in.ChatID)
	if err != nil {
		return err
	}
	if c == nil || c.State == nil {
		return fmt.Errorf("missing app state")
	}
	return c.State.SetChatEnabledByID(chatID, in.SetEnabled)
}

func (c *ConfigSetters) SetChatProcessToolsEnabled(in SetChatProcessToolsEnabledInput) error {
	chatID, err := parseChatID(in.ChatID)
	if err != nil {
		return err
	}
	if c == nil || c.State == nil {
		return fmt.Errorf("missing app state")
	}
	return c.State.SetChatProcessToolsEnabledByID(chatID, in.SetProcessToolsEnabled)
}

func (c *ConfigSetters) SetChatInteractiveInterruptEnabled(in SetChatInteractiveInterruptEnabledInput) error {
	chatID, err := parseChatID(in.ChatID)
	if err != nil {
		return err
	}
	if c == nil || c.State == nil {
		return fmt.Errorf("missing app state")
	}
	return c.State.SetChatInteractiveInterruptEnabledByID(chatID, in.SetInteractiveInterruptEnabled)
}

func (c *ConfigSetters) SetChatGPUs(in SetChatGPUsInput) error {
	chatID, err := parseChatID(in.ChatID)
	if err != nil {
		return err
	}
	if c == nil || c.State == nil {
		return fmt.Errorf("missing app state")
	}
	return c.State.SetChatGPUsByID(chatID, in.SetGPUs)
}

func (c *ConfigSetters) SetChatWorkspaceHostPath(in SetChatWorkspaceHostPathInput) error {
	chatID, err := parseChatID(in.ChatID)
	if err != nil {
		return err
	}
	if c == nil || c.State == nil {
		return fmt.Errorf("missing app state")
	}
	return c.State.SetChatWorkspaceHostPathByID(chatID, in.SetWorkspaceHostPath)
}

func (c *ConfigSetters) SetChatCodexProfileHostPath(in SetChatCodexProfileHostPathInput) error {
	chatID, err := parseChatID(in.ChatID)
	if err != nil {
		return err
	}
	if c == nil || c.State == nil {
		return fmt.Errorf("missing app state")
	}
	return c.State.SetChatCodexProfileHostPathByID(chatID, in.SetCodexProfileHostPath)
}

func (c *ConfigSetters) SetChatHostbridgeAliasCommand(in SetChatHostbridgeAliasCommandInput) error {
	chatID, err := parseChatID(in.ChatID)
	if err != nil {
		return err
	}
	command, err := c.chatHostbridgeAliasCommand(chatID, in.Alias)
	if err != nil {
		return err
	}
	command.Name = strings.TrimSpace(in.SetCommand)
	return c.State.SetChatHostbridgeAllowedCommandByID(chatID, in.Alias, command)
}

func (c *ConfigSetters) SetChatHostbridgeAliasDir(in SetChatHostbridgeAliasDirInput) error {
	chatID, err := parseChatID(in.ChatID)
	if err != nil {
		return err
	}
	command, err := c.chatHostbridgeAliasCommand(chatID, in.Alias)
	if err != nil {
		return err
	}
	command.Dir = strings.TrimSpace(in.SetDir)
	return c.State.SetChatHostbridgeAllowedCommandByID(chatID, in.Alias, command)
}

func (c *ConfigSetters) SetChatHostbridgeAliasArgs(in SetChatHostbridgeAliasArgsInput) error {
	chatID, err := parseChatID(in.ChatID)
	if err != nil {
		return err
	}
	command, err := c.chatHostbridgeAliasCommand(chatID, in.Alias)
	if err != nil {
		return err
	}
	args, err := parseConfigSetterArgsCSV(in.SetArgs)
	if err != nil {
		return err
	}
	command.Args = args
	return c.State.SetChatHostbridgeAllowedCommandByID(chatID, in.Alias, command)
}

func (c *ConfigSetters) SetChatHostbridgeAliasAllowExtraArgs(in SetChatHostbridgeAliasAllowExtraArgsInput) error {
	chatID, err := parseChatID(in.ChatID)
	if err != nil {
		return err
	}
	command, err := c.chatHostbridgeAliasCommand(chatID, in.Alias)
	if err != nil {
		return err
	}
	command.AllowExtraArgs = in.SetAllowExtraArgs
	return c.State.SetChatHostbridgeAllowedCommandByID(chatID, in.Alias, command)
}

func (c *ConfigSetters) SetChatHostbridgeAliasDelay(in SetChatHostbridgeAliasDelayInput) error {
	chatID, err := parseChatID(in.ChatID)
	if err != nil {
		return err
	}
	command, err := c.chatHostbridgeAliasCommand(chatID, in.Alias)
	if err != nil {
		return err
	}
	command.Delay = strings.TrimSpace(in.SetDelay)
	return c.State.SetChatHostbridgeAllowedCommandByID(chatID, in.Alias, command)
}

func (c *ConfigSetters) SetChatHostbridgeAliasRemoved(in SetChatHostbridgeAliasRemovedInput) error {
	chatID, err := parseChatID(in.ChatID)
	if err != nil {
		return err
	}
	if c == nil || c.State == nil {
		return fmt.Errorf("missing app state")
	}
	if !in.Remove {
		return nil
	}
	return c.State.RemoveChatHostbridgeAllowedCommandByID(chatID, in.Alias)
}

func (c *ConfigSetters) SetChatSkillAdded(in SetChatSkillAddedInput) error {
	chatID, err := parseChatID(in.ChatID)
	if err != nil {
		return err
	}
	if c == nil || c.State == nil {
		return fmt.Errorf("missing app state")
	}
	return c.State.AddChatSkillByID(chatID, strings.TrimSpace(in.AddSkill))
}

func (c *ConfigSetters) SetChatSkillRemoved(in SetChatSkillRemovedInput) error {
	chatID, err := parseChatID(in.ChatID)
	if err != nil {
		return err
	}
	if c == nil || c.State == nil {
		return fmt.Errorf("missing app state")
	}
	return c.State.RemoveChatSkillByID(chatID, strings.TrimSpace(in.RemoveSkill))
}

func (c *ConfigSetters) chatHostbridgeAliasCommand(chatID modeluuid.UUID, alias string) (hostbridgeserver.AllowedCommand, error) {
	if c == nil || c.State == nil {
		return hostbridgeserver.AllowedCommand{}, fmt.Errorf("missing app state")
	}
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return hostbridgeserver.AllowedCommand{}, fmt.Errorf("alias is empty")
	}
	command := c.State.ChatHostbridgeAllowedCommandsByID(chatID)[alias]
	return command, nil
}

func parseChatID(raw string) (modeluuid.UUID, error) {
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

func parseConfigSetterArgsCSV(raw string) ([]string, error) {
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

func (c *ConfigSetters) SetTelegramAdminUserID(in SetTelegramAdminUserIDInput) error {
	if c == nil || c.Local == nil {
		return fmt.Errorf("missing local config store")
	}
	return c.Local.PersistString("telegram.admin_user_id", strconv.FormatInt(in.SetTelegramAdminUserID, 10))
}
