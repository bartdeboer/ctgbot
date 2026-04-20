package appstate

import (
	"path"
	"strings"
)

func (c *Config) DockerImage() string {
	if c == nil || c.Store == nil {
		return "ctgbot-codex:latest"
	}
	return strings.TrimSpace(c.Store.GetString("docker.image", "ctgbot-codex:latest"))
}

func (c *Config) DockerCLIContainerName() string {
	if c == nil || c.Store == nil {
		return "ctgbot"
	}
	name := strings.TrimSpace(c.Store.GetString("docker.cli_container_name", "ctgbot"))
	if name == "" {
		return "ctgbot"
	}
	return name
}

func (c *Config) DockerDefaultWorkspaceHostPath() string {
	if c == nil || c.Store == nil {
		return ""
	}
	return absOrEmpty(c.Store.GetString("docker.workspace_host_path", ""))
}

func (c *Config) DockerContainerWorkspacePath() string {
	if c == nil || c.Store == nil {
		return normalizeContainerPath("", "/workspace")
	}
	return normalizeContainerPath(c.Store.GetString("docker.container_workspace_path", "/workspace"), "/workspace")
}

func (c *Config) DockerContainerHomePath() string {
	if c == nil || c.Store == nil {
		return normalizeContainerPath("", "/codex-home")
	}
	return normalizeContainerPath(c.Store.GetString("docker.container_home_path", "/codex-home"), "/codex-home")
}

func (c *Config) DockerContainerHostbridgeTLSDir() string {
	if c == nil || c.Store == nil {
		return normalizeContainerPath("", "/etc/ctgbot/hostbridge-tls")
	}
	return normalizeContainerPath(c.Store.GetString("docker.container_hostbridge_tls_dir", "/etc/ctgbot/hostbridge-tls"), "/etc/ctgbot/hostbridge-tls")
}

func normalizeContainerPath(raw string, fallback string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		v = fallback
	}
	v = strings.ReplaceAll(v, "\\", "/")
	if !strings.HasPrefix(v, "/") {
		v = "/" + v
	}
	return path.Clean(v)
}
