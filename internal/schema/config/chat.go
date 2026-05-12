package config

import (
	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/configengine"
)

func ChatEnabled(cfg *appstate.Config) configengine.Item {
	return chatBool("chat.enabled", "Enable or disable the current chat", cfg,
		func(chat appstate.ChatConfig) bool { return chat.Enabled() },
		func(chat appstate.ChatConfig, value bool) error { return chat.SetEnabled(value) },
		rootOrElevated(),
	)
}

func ChatProcessToolsEnabled(cfg *appstate.Config) configengine.Item {
	return chatBool("chat.process-tools-enabled", "Enable process tools for the current chat", cfg,
		func(chat appstate.ChatConfig) bool { return chat.ProcessToolsEnabled() },
		func(chat appstate.ChatConfig, value bool) error { return chat.SetProcessToolsEnabled(value) },
		rootOrElevated(),
	)
}

func ChatInteractiveInterruptEnabled(cfg *appstate.Config) configengine.Item {
	return chatBool("chat.interactive-interrupt-enabled", "Enable interactive interrupts for the current chat", cfg,
		func(chat appstate.ChatConfig) bool { return chat.InteractiveInterruptEnabled() },
		func(chat appstate.ChatConfig, value bool) error { return chat.SetInteractiveInterruptEnabled(value) },
		rootOrElevated(),
	)
}

func ChatContainerUserMode(cfg *appstate.Config) configengine.Item {
	return chatString("chat.container-user-mode", "Container user mode: default, host, or root", configengine.ValueString, cfg,
		func(chat appstate.ChatConfig) string { return chat.ContainerUserMode() },
		func(chat appstate.ChatConfig, value string) error { return chat.SetContainerUserMode(value) },
		rootOrElevated(),
	)
}

func ChatWorkspaceHostPath(cfg *appstate.Config) configengine.Item {
	return chatString("chat.workspace-host-path", "Workspace host path for the current chat", configengine.ValueString, cfg,
		func(chat appstate.ChatConfig) string { return chat.WorkspaceHostPath() },
		func(chat appstate.ChatConfig, value string) error { return chat.SetWorkspaceHostPath(value) },
		rootOrElevated(),
	)
}

func ChatCodexProfileHostPath(cfg *appstate.Config) configengine.Item {
	return chatString("chat.codex-profile-host-path", "Codex profile host path for the current chat", configengine.ValueString, cfg,
		func(chat appstate.ChatConfig) string { return chat.CodexProfileHostPath() },
		func(chat appstate.ChatConfig, value string) error { return chat.SetCodexProfileHostPath(value) },
		rootOrElevated(),
	)
}

func ChatSkills(cfg *appstate.Config) configengine.Item {
	return chatStringList("chat.skills", "Skill directories for the current chat", cfg,
		func(chat appstate.ChatConfig) []string { return chat.Skills() },
		func(chat appstate.ChatConfig, value []string) error { return chat.SetSkills(value) },
		rootOrElevated(),
	)
}

func ChatAgentDBAccessEnabled(cfg *appstate.Config) configengine.Item {
	return chatBoolWithPolicies("chat.enable-agent-db-access", "Enable trusted agent SQL access to the ctgbot database for the current chat", cfg,
		func(chat appstate.ChatConfig) bool { return chat.AgentDBAccessEnabled() },
		func(chat appstate.ChatConfig, value bool) error { return chat.SetAgentDBAccessEnabled(value) },
		rootOrElevated(),
		rootOnly(),
	)
}
