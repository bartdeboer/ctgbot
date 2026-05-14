package commands

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/message/mediafile"
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

type SendPayload struct {
	Payload message.OutboundPayload
}

func HostbridgeCommands() []commandengine.Definition {
	return []commandengine.Definition{
		RunCommandDefinition(),
		{
			Pattern:               "message <text>",
			Help:                  "Send a chat message with optional attachments",
			Build:                 buildMessagePayload,
			Sources:               []commandengine.Source{commandengine.SourceHostbridge},
			Policy:                agentPolicy(),
			InstructionVisibility: commandengine.InstructionEssential,
		},
		{
			Pattern:               "sendfile <path>",
			Help:                  "Upload a file",
			Build:                 buildSendFile,
			Sources:               []commandengine.Source{commandengine.SourceHostbridge},
			Policy:                agentPolicy(),
			InstructionVisibility: commandengine.InstructionEssential,
		},
		{
			Pattern:               "sendstdin",
			Help:                  "Send stdin as text",
			Build:                 buildSendStdin,
			Sources:               []commandengine.Source{commandengine.SourceHostbridge},
			Policy:                agentPolicy(),
			InstructionVisibility: commandengine.InstructionEssential,
		},
	}
}

func RunCommandDefinition() commandengine.Definition {
	return commandengine.Definition{
		Pattern:               "run <command>",
		Help:                  "Run a whitelisted host command",
		Build:                 buildRunCommand,
		Sources:               []commandengine.Source{commandengine.SourceHostbridge},
		Policy:                agentPolicy(),
		InstructionVisibility: commandengine.InstructionHidden,
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

type repeatPathFlag []string

func (f *repeatPathFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(*f, ",")
}

func (f *repeatPathFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	*f = append(*f, value)
	return nil
}

func buildMessagePayload(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("hostbridge message", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	contentType := fs.String("type", "", "Optional text content type")
	language := fs.String("language", "", "Optional legacy syntax hint")
	syntax := fs.String("syntax", "", "Optional syntax hint")
	var attach repeatPathFlag
	fs.Var(&attach, "attach", "Attachment descriptor; repeat for multiple attachments")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected message arguments: %s", strings.Join(fs.Args(), " "))
	}

	attachments := make([]message.Media, 0, len(attach))
	for _, raw := range attach {
		media, err := mediafile.ReadDescriptor(raw)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, media)
	}

	payload := message.OutboundPayload{
		Text: message.TextMessage{
			Text:        strings.TrimSpace(req.Params["text"]),
			ContentType: strings.TrimSpace(*contentType),
			Syntax:      resolveSyntax(*language, *syntax),
		},
		Attachments: attachments,
	}
	if payload.IsZero() {
		return nil, fmt.Errorf("message requires text or --attach")
	}
	return SendPayload{Payload: payload}, nil
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
