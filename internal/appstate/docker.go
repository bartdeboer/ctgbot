package appstate

import (
	"fmt"
	"path"
	"strings"
)

func (c *Config) Docker() DockerConfig {
	return DockerConfig{cfg: c}
}

type DockerConfig struct {
	cfg *Config
}

func (d DockerConfig) Image() string {
	return d.cfg.string("docker.image", "ctgbot-codex:latest")
}

func (d DockerConfig) SetImage(image string) error {
	return d.cfg.persistString("docker.image", strings.TrimSpace(image))
}

func (d DockerConfig) Dockerfile() string {
	name := strings.TrimSpace(d.cfg.string("docker.dockerfile", "Dockerfile"))
	if name == "" {
		return "Dockerfile"
	}
	return name
}

func (d DockerConfig) SetDockerfile(name string) error {
	name = strings.TrimSpace(strings.ReplaceAll(name, "\\", "/"))
	if name == "" {
		name = "Dockerfile"
	}
	if strings.Contains(name, "/") || name == "." || name == ".." {
		return fmt.Errorf("dockerfile must be a file name in the build context root: %s", name)
	}
	return d.cfg.persistString("docker.dockerfile", name)
}

func (d DockerConfig) DefaultWorkspaceHostPath() string {
	return absOrEmpty(d.cfg.string("docker.workspace_host_path", ""))
}

func (d DockerConfig) SetDefaultWorkspaceHostPath(raw string) error {
	resolved, err := d.cfg.ResolveWorkspaceHostPath(raw)
	if err != nil {
		return err
	}
	return d.cfg.persistString("docker.workspace_host_path", resolved)
}

func (d DockerConfig) ContainerWorkspacePath() string {
	return normalizeContainerPath(d.cfg.string("docker.container_workspace_path", "/workspace"), "/workspace")
}

func (d DockerConfig) ContainerHomePath() string {
	return normalizeContainerPath(d.cfg.string("docker.container_home_path", "/codex-home"), "/codex-home")
}

func (d DockerConfig) ContainerHostbridgeTLSDir() string {
	return normalizeContainerPath(d.cfg.string("docker.container_hostbridge_tls_dir", "/etc/ctgbot/hostbridge-tls"), "/etc/ctgbot/hostbridge-tls")
}

func (d DockerConfig) ContainerHostbridgeTCPAddr() string {
	addr := d.cfg.string("docker.container_hostbridge_tcp_addr", "host.docker.internal:4567")
	if addr == "" {
		return "host.docker.internal:4567"
	}
	return addr
}

func (d DockerConfig) SetContainerHostbridgeTCPAddr(addr string) error {
	return d.cfg.persistString("docker.container_hostbridge_tcp_addr", strings.TrimSpace(addr))
}

func normalizeContainerPath(raw string, fallback string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = fallback
	}
	value = strings.ReplaceAll(value, "\\", "/")
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	return path.Clean(value)
}
