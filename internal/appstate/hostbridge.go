package appstate

import "strings"

import hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"

func (c *Config) Hostbridge() HostbridgeConfig {
	return HostbridgeConfig{cfg: c}
}

type HostbridgeConfig struct {
	cfg *Config
}

func (h HostbridgeConfig) TLSRoot() string {
	if h.cfg == nil {
		return ""
	}
	return h.cfg.Profile().TLSRoot()
}

func (h HostbridgeConfig) TCPListenAddr() string {
	addr := h.cfg.string("hostbridge.tcp_listen_addr", "127.0.0.1:4567")
	if addr == "" {
		return "127.0.0.1:4567"
	}
	return addr
}

func (h HostbridgeConfig) ConfiguredTCPListenAddr() string {
	if h.cfg == nil {
		return ""
	}
	return strings.TrimSpace(h.cfg.string("hostbridge.tcp_listen_addr", ""))
}

func (h HostbridgeConfig) SetTCPListenAddr(addr string) error {
	return h.cfg.persistString("hostbridge.tcp_listen_addr", addr)
}

func (h HostbridgeConfig) RemoteListenAddr() string {
	if h.cfg == nil {
		return ""
	}
	return strings.TrimSpace(h.cfg.string("hostbridge.remote_listen_addr", ""))
}

func (h HostbridgeConfig) SetRemoteListenAddr(addr string) error {
	return h.cfg.persistString("hostbridge.remote_listen_addr", strings.TrimSpace(addr))
}

func (h HostbridgeConfig) ResolveAliases(clientIdentity string) map[string]hostbridgeserver.Alias {
	if h.cfg == nil {
		return hostbridgeserver.DefaultAliases()
	}
	chatID, ok := h.cfg.ParseChatClientIdentity(clientIdentity)
	if !ok {
		return hostbridgeserver.DefaultAliases()
	}
	return hostbridgeserver.MergeAliases(h.cfg.Chat(chatID).Hostbridge().Aliases())
}
