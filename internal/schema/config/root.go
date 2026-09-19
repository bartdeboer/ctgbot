package config

import (
	"fmt"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configengine"
)

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
	get := func(ctx commandengine.Context) (configengine.Value, error) {
		if cfg == nil {
			return configengine.Value{}, fmt.Errorf("missing config")
		}
		timeout, err := cfg.Codex().SessionTimeout()
		if err != nil {
			return configengine.Value{}, err
		}
		return configengine.String(timeout.String()), nil
	}
	return configengine.Item{
		Key:         "codex.session-timeout",
		Help:        "Codex turn timeout (default 0: no deadline; bare numbers are minutes)",
		Scope:       configengine.ScopeRoot,
		ValueType:   configengine.ValueDuration,
		ReadPolicy:  rootAgentOrElevated(),
		WritePolicy: rootOrElevated(),
		Get:         get,
		Set: func(ctx commandengine.Context, value configengine.Value) (configengine.Value, error) {
			if cfg == nil {
				return configengine.Value{}, fmt.Errorf("missing config")
			}
			if err := cfg.Codex().SetSessionTimeout(value.String()); err != nil {
				return configengine.Value{}, err
			}
			return get(ctx)
		},
	}
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
