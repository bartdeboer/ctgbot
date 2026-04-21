package chatcommandsv3

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

func buildSendFile(path string, caption string, contentType string) (SendFile, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return SendFile{}, fmt.Errorf("missing file path")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return SendFile{}, err
	}
	return SendFile{
		Filename:    filepath.Base(path),
		Caption:     strings.TrimSpace(caption),
		ContentType: strings.TrimSpace(contentType),
		Content:     content,
	}, nil
}

func buildSendText(text string, contentType string, fenced bool, legacyLanguage string, syntax string) SendText {
	language := strings.TrimSpace(syntax)
	if language == "" {
		language = strings.TrimSpace(legacyLanguage)
	}
	if language != "" {
		fenced = true
	}
	return SendText{
		Text:        text,
		ContentType: strings.TrimSpace(contentType),
		Fenced:      fenced,
		Language:    language,
	}
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
