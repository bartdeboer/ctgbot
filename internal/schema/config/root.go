package config

import (
	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/configengine"
)

func TelegramToken(cfg *appstate.Config) configengine.Item {
	return rootString("telegram.token", "Telegram bot token", configengine.ValueString, cfg,
		func(cfg *appstate.Config) string { return cfg.Telegram().Token() },
		func(cfg *appstate.Config, value string) error { return cfg.Telegram().SetToken(value) },
		rootOnly(), rootOnly(),
	)
}

func TelegramPollTimeout(cfg *appstate.Config) configengine.Item {
	return rootString("telegram.poll-timeout", "Telegram long-poll timeout", configengine.ValueDuration, cfg,
		func(cfg *appstate.Config) string { return cfg.Telegram().PollTimeout().String() },
		func(cfg *appstate.Config, value string) error { return cfg.Telegram().SetPollTimeout(value) },
		rootOnly(), rootOnly(),
	)
}

func TelegramDebounceWindow(cfg *appstate.Config) configengine.Item {
	return rootString("telegram.debounce-window", "Telegram message debounce window", configengine.ValueDuration, cfg,
		func(cfg *appstate.Config) string { return cfg.Telegram().DebounceWindow().String() },
		func(cfg *appstate.Config, value string) error { return cfg.Telegram().SetDebounceWindow(value) },
		rootOnly(), rootOnly(),
	)
}

func TelegramRenderFormat(cfg *appstate.Config) configengine.Item {
	return rootString("telegram.render-format", "Telegram outbound render format", configengine.ValueString, cfg,
		func(cfg *appstate.Config) string { return cfg.Telegram().RenderFormat() },
		func(cfg *appstate.Config, value string) error { return cfg.Telegram().SetRenderFormat(value) },
		rootOnly(), rootOnly(),
	)
}

func BuildCompilerPath(cfg *appstate.Config) configengine.Item {
	return rootString("build.compiler-path", "Compiler path used for local builds", configengine.ValueString, cfg,
		func(cfg *appstate.Config) string { return cfg.Global().BuildCompilerPath() },
		func(cfg *appstate.Config, value string) error { return cfg.Global().SetBuildCompilerPath(value) },
		rootOnly(), rootOnly(),
	)
}

func GitUserName(cfg *appstate.Config) configengine.Item {
	return rootString("git.user_name", "Git author/committer name for sandbox commits", configengine.ValueString, cfg,
		func(cfg *appstate.Config) string { return cfg.Git().UserName() },
		func(cfg *appstate.Config, value string) error { return cfg.Git().SetUserName(value) },
		rootOnly(), rootOnly(),
	)
}

func GitUserEmail(cfg *appstate.Config) configengine.Item {
	return rootString("git.user_email", "Git author/committer email for sandbox commits", configengine.ValueString, cfg,
		func(cfg *appstate.Config) string { return cfg.Git().UserEmail() },
		func(cfg *appstate.Config, value string) error { return cfg.Git().SetUserEmail(value) },
		rootOnly(), rootOnly(),
	)
}

func CodexSessionTimeout(cfg *appstate.Config) configengine.Item {
	return rootString("codex.session-timeout", "Codex session timeout", configengine.ValueDuration, cfg,
		func(cfg *appstate.Config) string { return cfg.Codex().SessionTimeout().String() },
		func(cfg *appstate.Config, value string) error { return cfg.Codex().SetSessionTimeout(value) },
		rootAgentOrElevated(), rootOrElevated(),
	)
}

func CodexModel(cfg *appstate.Config) configengine.Item {
	return rootString("codex.model", "Codex model", configengine.ValueString, cfg,
		func(cfg *appstate.Config) string { return cfg.Codex().Model() },
		func(cfg *appstate.Config, value string) error { return cfg.Codex().SetModel(value) },
		rootOrAgent(), rootOnly(),
	)
}

func CodexProfileHostPath(cfg *appstate.Config) configengine.Item {
	return rootString("codex.profile-host-path", "Codex profile host path", configengine.ValueString, cfg,
		func(cfg *appstate.Config) string { return cfg.Codex().ProfileHostPath() },
		func(cfg *appstate.Config, value string) error { return cfg.Codex().SetProfileHostPath(value) },
		rootOnly(), rootOnly(),
	)
}

func CodexCLIHomePath(cfg *appstate.Config) configengine.Item {
	return rootString("codex.cli-home-path", "Legacy alias for the Codex profile host path", configengine.ValueString, cfg,
		func(cfg *appstate.Config) string { return cfg.Codex().ProfileHostPath() },
		func(cfg *appstate.Config, value string) error { return cfg.Codex().SetProfileHostPath(value) },
		rootOnly(), rootOnly(),
	)
}

func CodexSharedHomePath(cfg *appstate.Config) configengine.Item {
	return rootString("codex.shared-home-path", "Legacy alias for the Codex profile host path", configengine.ValueString, cfg,
		func(cfg *appstate.Config) string { return cfg.Codex().ProfileHostPath() },
		func(cfg *appstate.Config, value string) error { return cfg.Codex().SetProfileHostPath(value) },
		rootOnly(), rootOnly(),
	)
}

func CodexLoginCallbackPort() configengine.Item {
	return rootReadOnlyInt("codex.login-callback-port", "Codex login callback port", appstate.CodexLoginCallbackPort, rootOnly())
}

func DockerImage(cfg *appstate.Config) configengine.Item {
	return rootString("docker.image", "Docker image used for ctgbot runtime containers", configengine.ValueString, cfg,
		func(cfg *appstate.Config) string { return cfg.Docker().Image() },
		func(cfg *appstate.Config, value string) error { return cfg.Docker().SetImage(value) },
		rootOrAgent(), rootOnly(),
	)
}

func Dockerfile(cfg *appstate.Config) configengine.Item {
	return rootString("docker.dockerfile", "Dockerfile used to build the agent image", configengine.ValueString, cfg,
		func(cfg *appstate.Config) string { return cfg.Docker().Dockerfile() },
		func(cfg *appstate.Config, value string) error { return cfg.Docker().SetDockerfile(value) },
		rootOrAgent(), rootOnly(),
	)
}

func DockerWorkspaceHostPath(cfg *appstate.Config) configengine.Item {
	return rootString("docker.workspace-host-path", "Default workspace host path", configengine.ValueString, cfg,
		func(cfg *appstate.Config) string { return cfg.Docker().DefaultWorkspaceHostPath() },
		func(cfg *appstate.Config, value string) error { return cfg.Docker().SetDefaultWorkspaceHostPath(value) },
		rootOnly(), rootOnly(),
	)
}

func DockerContainerHostbridgeTCPAddr(cfg *appstate.Config) configengine.Item {
	return rootString("docker.container-hostbridge-tcp-addr", "Hostbridge TCP address from inside containers", configengine.ValueString, cfg,
		func(cfg *appstate.Config) string { return cfg.Docker().ContainerHostbridgeTCPAddr() },
		func(cfg *appstate.Config, value string) error {
			return cfg.Docker().SetContainerHostbridgeTCPAddr(value)
		},
		rootOnly(), rootOnly(),
	)
}

func HostbridgeTCPListenAddr(cfg *appstate.Config) configengine.Item {
	return rootString("hostbridge.tcp-listen-addr", "Hostbridge TCP listen address", configengine.ValueString, cfg,
		func(cfg *appstate.Config) string { return cfg.Hostbridge().TCPListenAddr() },
		func(cfg *appstate.Config, value string) error { return cfg.Hostbridge().SetTCPListenAddr(value) },
		rootOnly(), rootOnly(),
	)
}
