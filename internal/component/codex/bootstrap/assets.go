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
	AgentHome                 string
	CodexProfile              string
	ContainerOS               string
	HostOS                    string
	HostbridgeAddr            string
	BinariesSynopsis          string
	HostbridgeControlSynopsis string
	HostbridgeExamples        []string
	ChatProvider              string
	MessagePrefix             string
	KeepRepliesConcise        bool
	RuntimeNotices            []string
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
