package appstate

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

func (h HostbridgeConfig) SetTCPListenAddr(addr string) error {
	return h.cfg.persistString("hostbridge.tcp_listen_addr", addr)
}

func (h HostbridgeConfig) ResolveAllowedCommands(clientIdentity string) map[string]hostbridgeserver.AllowedCommand {
	if h.cfg == nil {
		return hostbridgeserver.DefaultAllowedCommands()
	}
	chatID, ok := h.cfg.ParseChatClientIdentity(clientIdentity)
	if !ok {
		return hostbridgeserver.DefaultAllowedCommands()
	}
	return hostbridgeserver.MergeNamedAllowedCommands(h.cfg.Chat(chatID).Hostbridge().AllowedCommands())
}
