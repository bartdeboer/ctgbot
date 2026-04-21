package appstate

import (
	"path/filepath"
	"strings"

	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
)

func (c *Config) ResolveHostbridgeAllowedCommands(clientIdentity string) map[string]hostbridgeserver.AllowedCommand {
	if c == nil {
		return hostbridgeserver.DefaultAllowedCommands()
	}
	chatID, ok := c.ParseChatClientIdentity(clientIdentity)
	if !ok {
		return hostbridgeserver.DefaultAllowedCommands()
	}
	return hostbridgeserver.MergeNamedAllowedCommands(c.ChatHostbridgeAllowedCommandsByID(chatID))
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
