package commands

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type RunCommand struct {
	Command string
	Args    []string
	Stdin   []byte
	Timeout int
}

type SendMedia struct {
	Filename    string
	Caption     string
	ContentType string
	Syntax      string
	Content     []byte
}

func HostbridgeCommands() []commandengine.Definition {
	return []commandengine.Definition{
		RunCommandDefinition(),
		{
			ID:      "hostbridge.sendfile",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  agentPolicy(),
			Routes: []commandengine.Route{{
				Pattern: "sendfile <path>",
				Help:    "Upload a file",
				Build:   buildSendFile,
			}},
		},
		{
			ID:      "hostbridge.sendstdin",
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  agentPolicy(),
			Routes: []commandengine.Route{{
				Pattern: "sendstdin",
				Help:    "Send stdin as text",
				Build:   buildSendStdin,
			}},
		},
	}
}

func RunCommandDefinition() commandengine.Definition {
	return commandengine.Definition{
		ID:      "hostbridge.run",
		Sources: []commandengine.Source{commandengine.SourceHostbridge},
		Policy:  agentPolicy(),
		Routes: []commandengine.Route{{
			Pattern: "run <command>",
			Help:    "Run a whitelisted host command",
			Build:   buildRunCommand,
		}},
	}
}

func buildRunCommand(req *clir.Request) (any, error) {
	command := strings.TrimSpace(req.Params["command"])
	if command == "" {
		return nil, fmt.Errorf("missing command")
	}
	return RunCommand{
		Command: command,
		Args:    append([]string{}, req.Extra...),
		Timeout: 30,
	}, nil
}

func buildSendFile(req *clir.Request) (any, error) {
	caption, contentType, syntax, err := parseSendMediaFlags("hostbridge sendfile", req.Extra)
	if err != nil {
		return nil, err
	}
	path := strings.TrimSpace(req.Params["path"])
	if path == "" {
		return nil, fmt.Errorf("missing file path")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(contentType) == "" && strings.TrimSpace(syntax) != "" {
		contentType = "text/plain"
	}
	return SendMedia{
		Filename:    filepath.Base(path),
		Caption:     caption,
		ContentType: strings.TrimSpace(contentType),
		Syntax:      strings.TrimSpace(syntax),
		Content:     append([]byte(nil), content...),
	}, nil
}

func buildSendStdin(req *clir.Request) (any, error) {
	caption, contentType, syntax, err := parseSendMediaFlags("hostbridge sendstdin", req.Extra)
	if err != nil {
		return nil, err
	}
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	if strings.TrimSpace(contentType) == "" {
		contentType = "text/plain"
	}
	return SendMedia{
		Filename:    "stdin.txt",
		Caption:     caption,
		ContentType: strings.TrimSpace(contentType),
		Syntax:      strings.TrimSpace(syntax),
		Content:     append([]byte(nil), stdin...),
	}, nil
}

func parseSendMediaFlags(name string, args []string) (caption string, contentType string, syntax string, err error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	captionFlag := fs.String("caption", "", "Optional caption")
	contentTypeFlag := fs.String("type", "", "Optional content type")
	languageFlag := fs.String("language", "", "Optional legacy syntax hint")
	syntaxFlag := fs.String("syntax", "", "Optional syntax hint")
	if err := fs.Parse(args); err != nil {
		return "", "", "", err
	}
	return strings.TrimSpace(*captionFlag), strings.TrimSpace(*contentTypeFlag), resolveSyntax(*languageFlag, *syntaxFlag), nil
}

func resolveSyntax(legacyLanguage string, syntax string) string {
	syntax = strings.TrimSpace(syntax)
	if syntax != "" {
		return syntax
	}
	return strings.TrimSpace(legacyLanguage)
}

func agentPolicy() simplerbac.Rule {
	return simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent)
}
