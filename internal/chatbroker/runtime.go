package chatbroker

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/agent"
	"github.com/bartdeboer/ctgbot/internal/bootstrapassets"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridgetls"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

func (b *Broker) prepareRuntime(ctx context.Context, conv *Thread, forceSetup bool) (agent.Agent, *sandboxengine.Sandbox, error) {
	if err := b.ensureSandboxRuntime(ctx, conv); err != nil {
		return nil, nil, err
	}
	agent, err := b.agent(conv.AgentProviderType)
	if err != nil {
		return nil, nil, err
	}
	sbx := b.sandboxForThread(conv)
	if forceSetup || !conv.Initialized {
		if err := agent.SetupEnvironment(ctx, sbx); err != nil {
			return nil, nil, err
		}
		if err := b.installConfiguredSkills(ctx, conv.ChatID, agent, sbx); err != nil {
			return nil, nil, err
		}
		conv.Initialized = true
		if b.Sessions != nil {
			_ = b.Sessions.SaveThread(ctx, conv)
		}
	}
	return agent, sbx, nil
}

func (b *Broker) installConfiguredSkills(ctx context.Context, chatID modeluuid.UUID, agentImpl agent.Agent, sbx *sandboxengine.Sandbox) error {
	if b.Config == nil || agentImpl == nil || sbx == nil {
		return nil
	}
	installer, ok := agentImpl.(agent.SkillInstallingAgent)
	if !ok {
		return nil
	}
	for _, skillDir := range b.Config.ChatSkillsByID(chatID) {
		if err := installer.InstallSkill(ctx, sbx, skillDir); err != nil {
			return err
		}
	}
	return nil
}

func (b *Broker) ensureSandboxRuntime(ctx context.Context, conv *Thread) error {
	if b.Config == nil {
		return fmt.Errorf("missing config")
	}
	if _, err := b.Config.EnsureChatRuntimePaths(conv.ChatID); err != nil {
		return err
	}
	if err := hostbridgetls.EnsureChatClientMaterials(
		b.Config.HostbridgeTLSRoot(),
		b.Config.ChatTLSDirByID(conv.ChatID),
		b.Config.ChatClientIdentity(conv.ChatID),
	); err != nil {
		return fmt.Errorf("ensure hostbridge tls client materials: %w", err)
	}
	return nil
}

func (b *Broker) developerInstructions(chatID modeluuid.UUID, conv *Thread) string {
	allowedCommands := append([]string{}, hostbridge.AllowedCommandNames(
		hostbridge.MergeNamedAllowedCommands(b.Config.ChatHostbridgeAllowedCommandsByID(chatID)),
	)...)
	sort.Strings(allowedCommands)

	allowedCommandsText := strings.Join(allowedCommands, ", ")
	if strings.TrimSpace(allowedCommandsText) == "" {
		allowedCommandsText = "<none>"
	}

	chatProvider := "Chat"
	messagePrefix := ""
	keepRepliesConcise := false

	if chatCfg, err := b.Config.FindChatByID(chatID); err == nil && chatCfg != nil {
		switch chatCfg.ProviderType {
		case "telegram":
			chatProvider = "Telegram"
			messagePrefix = "🤖"
			keepRepliesConcise = true
		default:
			chatProvider = strings.TrimSpace(chatCfg.ProviderType)
			if chatProvider == "" {
				chatProvider = "Chat"
			}
		}
	}

	text, err := bootstrapassets.Text(bootstrapassets.TemplateData{
		Workspace:          conv.ContainerWorkspace,
		WorkspaceInbox:     conv.ContainerWorkspace + "/inbox",
		CodexHome:          conv.ContainerHome,
		ContainerOS:        "linux",
		HostOS:             runtime.GOOS,
		HostbridgeAddr:     b.Config.DockerContainerHostbridgeTCPAddr(),
		Binaries:           allowedCommandsText,
		ChatProvider:       chatProvider,
		MessagePrefix:      messagePrefix,
		KeepRepliesConcise: keepRepliesConcise,
	})
	if err != nil {
		b.logf("render bootstrap template failed: %v", err)
		return ""
	}
	return text
}
