package copilot

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/agentcommon"
)

func prepareProfile(profile string) error {
	if strings.TrimSpace(profile) == "" {
		return fmt.Errorf("missing copilot profile path")
	}
	return os.MkdirAll(filepath.Join(profile, ".copilot"), 0700)
}

func stagePrompt(profile, runtimeProfile, text string) (string, func(), error) {
	if err := prepareProfile(profile); err != nil {
		return "", nil, err
	}
	// A unique directory per turn avoids overwriting another thread's prompt or
	// bootstrap. Both directories/files are private; no credential files are read.
	dir, err := os.MkdirTemp(profile, "copilot-turn-")
	if err != nil {
		return "", nil, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }
	if err := os.WriteFile(filepath.Join(dir, "prompt.txt"), []byte(text), 0600); err != nil {
		cleanup()
		return "", nil, err
	}
	return filepath.Join(runtimeProfile, filepath.Base(dir), "prompt.txt"), cleanup, nil
}

func bootstrap(workspace string, instructions component.TurnInstructions) string {
	commands := append([]string(nil), instructions.HostbridgeControlCommands...)
	if len(instructions.HostbridgeCommandNames) > 0 {
		commands = append(commands, "hostbridge run <alias> [args...]")
	}
	synopsis := commandengine.CommandSynopsis("hostbridge", commands, instructions.HostbridgeFamilyDescriptions)
	lines := []string{
		"You are GitHub Copilot CLI running inside ctgbot's Linux Docker sandbox.",
		"Persisted personal workspace: /home/agent; /home/agent/bin is on PATH.",
		"Shared workspace: " + workspace, "Workspace inbox: " + workspace + "/inbox",
		"For persistent services use supervisorctl; run supervisorctl --help for usage.",
		"Use only the available Hostbridge commands; the broker owns host authority.",
		"When messaging threads, end your turn to receive their response. Do not poll for replies.",
		"Do not add Co-Authored-By trailers unless explicitly requested.",
		"Use hostbridge turn info for input metadata and hostbridge sendfile for artifacts.",
	}
	if instructions.MessagePrefix != "" {
		lines = append(lines, "Start every final assistant message with "+instructions.MessagePrefix)
	}
	if instructions.KeepRepliesConcise {
		lines = append(lines, "Keep replies concise.")
	}
	lines = append(lines, "Canonical Hostbridge commands:", synopsis, "Available host aliases:", commandengine.CommandSynopsis("hostbridge run", instructions.HostbridgeCommandNames))
	lines = append(lines, agentcommon.HostbridgeExampleLines(synopsis)...)
	lines = append(lines, instructions.RuntimeNotices...)
	return strings.Join(lines, "\n")
}
