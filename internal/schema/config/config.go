package config

import (
	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/configengine"
)

func Registry(cfg *appstate.Config) (*configengine.Registry, error) {
	return configengine.NewRegistry(Items(cfg)...)
}

func Items(cfg *appstate.Config) []configengine.Item {
	return []configengine.Item{
		BuildCompilerPath(cfg),
		ChatEnabled(cfg),
		ChatProcessToolsEnabled(cfg),
		ChatInteractiveInterruptEnabled(cfg),
		ChatContainerUserMode(cfg),
		ChatWorkspaceHostPath(cfg),
		ChatCodexProfileHostPath(cfg),
		ChatSkills(cfg),
		CodexCLIHomePath(cfg),
		CodexLoginCallbackPort(),
		CodexModel(cfg),
		CodexProfileHostPath(cfg),
		CodexSessionTimeout(cfg),
		CodexSharedHomePath(cfg),
		DockerContainerHostbridgeTCPAddr(cfg),
		Dockerfile(cfg),
		DockerImage(cfg),
		DockerWorkspaceHostPath(cfg),
		GitUserEmail(cfg),
		GitUserName(cfg),
		HostbridgeTCPListenAddr(cfg),
	}
}
