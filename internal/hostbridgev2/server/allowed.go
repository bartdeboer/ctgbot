package server

import legacyserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"

type AllowedCommand = legacyserver.AllowedCommand

func DefaultAllowedCommands() map[string]AllowedCommand {
	return legacyserver.DefaultAllowedCommands()
}

func MergeAllowedCommands(extra map[string]string) map[string]AllowedCommand {
	return legacyserver.MergeAllowedCommands(extra)
}

func StaticAllowedCommandResolver(allowed map[string]AllowedCommand) AllowedCommandResolver {
	return legacyserver.StaticAllowedCommandResolver(allowed)
}
