package admin

import (
	"flag"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/go-clir"
)

func FormatHelp(definitions []commandengine.Definition) string {
	var lines []string
	for _, definition := range definitions {
		for _, route := range definition.Routes() {
			if route.Hidden {
				continue
			}
			line := commandengine.NormalizePattern(route.Pattern)
			if help := strings.TrimSpace(definition.Help); help != "" {
				line += " - " + help
			}
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return "no component commands"
	}
	sort.Strings(lines)
	return strings.Join(lines, "\n")
}

func definitionsForSource(definitions []commandengine.Definition, source commandengine.Source) []commandengine.Definition {
	if source == "" {
		return definitions
	}
	out := make([]commandengine.Definition, 0, len(definitions))
	for _, definition := range definitions {
		if definition.AllowsSource(source) {
			out = append(out, definition)
		}
	}
	return out
}

func buildComponentHelp(req *clir.Request) (any, error) {
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component")
	}
	return ComponentHelpCommand{Component: componentRef}, nil
}

func buildAuthStatus(req *clir.Request) (any, error) {
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component")
	}
	if extra := strings.TrimSpace(strings.Join(req.Extra, " ")); extra != "" {
		return nil, fmt.Errorf("unexpected auth status arguments: %s", extra)
	}
	return AuthStatusCommand{Component: componentRef}, nil
}

func buildManagedFileList(req *clir.Request) (any, error) {
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component")
	}
	return ManagedFileListCommand{Component: componentRef}, nil
}

func buildManagedFileStatus(req *clir.Request) (any, error) {
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component")
	}
	return ManagedFileStatusCommand{Component: componentRef}, nil
}

func buildManagedFilePut(req *clir.Request) (any, error) {
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component")
	}
	file := strings.TrimSpace(req.Params["file"])
	if file == "" {
		return nil, fmt.Errorf("missing managed file")
	}
	fs := flag.NewFlagSet("component managed-file put", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	contentType := fs.String("type", "", "Optional content type")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	content, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return ManagedFilePutCommand{
		Component:   componentRef,
		File:        file,
		ContentType: strings.TrimSpace(*contentType),
		Content:     append([]byte(nil), content...),
	}, nil
}

type repeatStringFlag []string

func (f *repeatStringFlag) String() string {
	if f == nil {
		return ""
	}
	return strings.Join(*f, ",")
}

func (f *repeatStringFlag) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	*f = append(*f, value)
	return nil
}

func buildMessagesSend(req *clir.Request) (any, error) {
	componentRef := strings.TrimSpace(req.Params["component"])
	if componentRef == "" {
		return nil, fmt.Errorf("missing component")
	}
	fs := flag.NewFlagSet("component messages send", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var to repeatStringFlag
	var cc repeatStringFlag
	var bcc repeatStringFlag
	fs.Var(&to, "to", "Recipient email address; repeat for multiple recipients")
	fs.Var(&cc, "cc", "CC email address; repeat for multiple recipients")
	fs.Var(&bcc, "bcc", "BCC email address; repeat for multiple recipients")
	subject := fs.String("subject", "", "Message subject")
	threadID := fs.String("thread-id", "", "Gmail thread id for replies")
	inReplyTo := fs.String("in-reply-to", "", "RFC Message-ID being replied to")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	if len(fs.Args()) > 0 {
		return nil, fmt.Errorf("unexpected messages send arguments: %s", strings.Join(fs.Args(), " "))
	}
	content, err := io.ReadAll(os.Stdin)
	if err != nil {
		return nil, fmt.Errorf("read stdin: %w", err)
	}
	return MessagesSendCommand{
		Component: componentRef,
		To:        append([]string(nil), to...),
		Cc:        append([]string(nil), cc...),
		Bcc:       append([]string(nil), bcc...),
		Subject:   strings.TrimSpace(*subject),
		Body:      string(content),
		ThreadID:  strings.TrimSpace(*threadID),
		InReplyTo: strings.TrimSpace(*inReplyTo),
	}, nil
}
