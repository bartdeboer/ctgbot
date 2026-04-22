package chatcommands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func buildRunCommand(command string, args []string, stdin []byte) RunCommand {
	return RunCommand{
		Command: strings.TrimSpace(command),
		Args:    append([]string{}, args...),
		Stdin:   append([]byte(nil), stdin...),
		Timeout: 30,
	}
}

func buildSendMediaFile(path string, caption string, contentType string, legacyLanguage string, syntax string) (SendMedia, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return SendMedia{}, fmt.Errorf("missing file path")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return SendMedia{}, err
	}
	syntax = resolveSyntax(legacyLanguage, syntax)
	contentType = strings.TrimSpace(contentType)
	if contentType == "" && syntax != "" {
		contentType = "text/plain"
	}
	return SendMedia{
		Filename:    filepath.Base(path),
		Caption:     strings.TrimSpace(caption),
		ContentType: contentType,
		Syntax:      syntax,
		Content:     content,
	}, nil
}

func buildSendMediaStdin(text string, contentType string, legacyLanguage string, syntax string) SendMedia {
	syntax = resolveSyntax(legacyLanguage, syntax)
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = "text/plain"
	}
	return SendMedia{
		Filename:    "stdin.txt",
		ContentType: contentType,
		Syntax:      syntax,
		Content:     []byte(text),
	}
}

func resolveSyntax(legacyLanguage string, syntax string) string {
	syntax = strings.TrimSpace(syntax)
	if syntax != "" {
		return syntax
	}
	return strings.TrimSpace(legacyLanguage)
}

func buildConfigList() ConfigList {
	return ConfigList{}
}

func buildConfigSet(setting string, value string) (ConfigSet, error) {
	setting = strings.TrimSpace(setting)
	if setting == "" {
		return ConfigSet{}, fmt.Errorf("missing setting name")
	}
	return ConfigSet{Setting: setting, Value: value}, nil
}

func buildRefreshContainer() RefreshContainer { return RefreshContainer{} }

func buildPurgeChat() PurgeChat { return PurgeChat{} }

func buildInterruptTurn() InterruptTurn { return InterruptTurn{} }

func buildUpgrade() Upgrade { return Upgrade{} }

func buildQuit() Quit { return Quit{} }
