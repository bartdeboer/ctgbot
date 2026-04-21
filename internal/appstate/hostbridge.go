package appstate

import (
	"path/filepath"
	"strings"

	hostbridgev2server "github.com/bartdeboer/ctgbot/internal/hostbridgev2/server"
)

func (c *Config) ResolveHostbridgeAllowedCommands(clientIdentity string) map[string]hostbridgev2server.AllowedCommand {
	if c == nil {
		return hostbridgev2server.DefaultAllowedCommands()
	}
	chatID, ok := c.ParseChatClientIdentity(clientIdentity)
	if !ok {
		return hostbridgev2server.DefaultAllowedCommands()
	}
	return hostbridgev2server.MergeNamedAllowedCommands(c.ChatHostbridgeAllowedCommandsByID(chatID))
}

func (c *Config) HostbridgeTLSRoot() string {
	return filepath.Join(c.Root(), "tls")
}

func (c *Config) HostbridgeTCPListenAddr() string {
	if c == nil || c.Store == nil {
		return "127.0.0.1:4567"
	}
	v := strings.TrimSpace(c.Store.GetString("hostbridge.tcp_listen_addr", "127.0.0.1:4567"))
	if v == "" {
		return "127.0.0.1:4567"
	}
	return v
}

func (c *Config) DockerContainerHostbridgeTCPAddr() string {
	if c == nil || c.Store == nil {
		return "host.docker.internal:4567"
	}
	v := strings.TrimSpace(c.Store.GetString("docker.container_hostbridge_tcp_addr", "host.docker.internal:4567"))
	if v == "" {
		return "host.docker.internal:4567"
	}
	return v
}
