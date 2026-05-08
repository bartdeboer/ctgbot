package bootstrap

import (
	"bytes"
	_ "embed"
	"strings"
	"text/template"
)

//go:embed BOOTSTRAP.md
var bootstrapText string

type TemplateData struct {
	Workspace                 string
	WorkspaceInbox            string
	CodexHome                 string
	ContainerOS               string
	HostOS                    string
	HostbridgeAddr            string
	Binaries                  string
	HostbridgeControlCommands []string
	ChatProvider              string
	MessagePrefix             string
	KeepRepliesConcise        bool
}

func Text(data TemplateData) (string, error) {
	tpl, err := template.New("bootstrap").Parse(bootstrapText)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}
