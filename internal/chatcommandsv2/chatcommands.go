package chatcommandsv2

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	clir "github.com/bartdeboer/go-clir"
)

type Operation string

const (
	OpRunCommand Operation = "run-command"
	OpSendFile   Operation = "send-file"
	OpSendText   Operation = "send-text"
	OpConfigList Operation = "config-list"
	OpConfigSet  Operation = "config-set"
)

type Request struct {
	Op      Operation
	Command string
	Args    []string
	Stdin   []byte
	Cwd     string
	Env     map[string]string
	Timeout int

	Filename    string
	Caption     string
	ContentType string
	Content     []byte

	Text     string
	Fenced   bool
	Language string

	Setting string
	Value   string
}

type HostbridgeRequest struct {
	SandboxID string
	Request   Request
}

type Runner interface {
	Execute(req Request) error
}

type ChatCommands struct {
	router *clir.Router
	runner Runner
}

func New(runner Runner) *ChatCommands {
	c := &ChatCommands{runner: runner}
	c.router = c.newRouter()
	return c
}

func (c *ChatCommands) Router() *clir.Router {
	if c == nil {
		return nil
	}
	return c.router
}

func (c *ChatCommands) Run(ctx context.Context, argv []string) error {
	if c == nil || c.router == nil {
		return fmt.Errorf("chat commands are unavailable")
	}
	return c.router.Run(ctx, argv)
}

func (c *ChatCommands) newRouter() *clir.Router {
	r := clir.New()
	r.Routes(func(b *clir.Builder) {
		b.Handle("run <command>", "Run a whitelisted host command", func(req *clir.Request) error {
			stdinData, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			payload := buildRunCommandRequest(req.Params["command"], req.Extra, stdinData)
			return c.execute(payload)
		})

		b.Handle("sendfile <path>", "Upload a file", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chatcommands sendfile", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			caption := fs.String("caption", "", "Optional caption")
			contentType := fs.String("type", "", "Optional content type")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			payload, err := buildSendFileRequest(req.Params["path"], strings.TrimSpace(*caption), strings.TrimSpace(*contentType))
			if err != nil {
				return err
			}
			return c.execute(payload)
		})

		b.Handle("config list", "List config", func(req *clir.Request) error {
			return c.execute(buildConfigListRequest())
		})

		b.Handle("config set <name> <value>", "Set config", func(req *clir.Request) error {
			payload, err := buildConfigSetRequest(req.Params["name"], req.Params["value"])
			if err != nil {
				return err
			}
			return c.execute(payload)
		})

		b.Handle("sendstdin", "Send stdin as text", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chatcommands sendstdin", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			fenced := fs.Bool("fenced", false, "Wrap text in a fenced block")
			language := fs.String("language", "", "Optional legacy fence language")
			contentType := fs.String("type", "text/plain", "Optional content type")
			syntax := fs.String("syntax", "", "Optional syntax hint")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			stdinData, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			payload, err := buildSendTextRequest(string(stdinData), strings.TrimSpace(*contentType), *fenced, strings.TrimSpace(*language), strings.TrimSpace(*syntax))
			if err != nil {
				return err
			}
			return c.execute(payload)
		})
	})
	return r
}

func (c *ChatCommands) execute(req Request) error {
	if c == nil || c.runner == nil {
		return fmt.Errorf("chat command runner is unavailable")
	}
	return c.runner.Execute(req)
}

func buildRunCommandRequest(command string, extra []string, stdin []byte) Request {
	return Request{
		Op:      OpRunCommand,
		Command: strings.TrimSpace(command),
		Args:    append([]string{}, extra...),
		Stdin:   append([]byte(nil), stdin...),
		Timeout: 30,
	}
}

func buildSendFileRequest(path string, caption string, contentType string) (Request, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Request{}, fmt.Errorf("missing file path")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return Request{}, err
	}
	return Request{
		Op:          OpSendFile,
		Filename:    filepath.Base(path),
		Caption:     caption,
		ContentType: contentType,
		Content:     content,
	}, nil
}

func buildSendTextRequest(text string, contentType string, fenced bool, legacyLanguage string, syntax string) (Request, error) {
	language := strings.TrimSpace(syntax)
	if language == "" {
		language = strings.TrimSpace(legacyLanguage)
	}
	if language != "" {
		fenced = true
	}
	return Request{
		Op:          OpSendText,
		Text:        text,
		ContentType: strings.TrimSpace(contentType),
		Fenced:      fenced,
		Language:    language,
	}, nil
}

func buildConfigListRequest() Request {
	return Request{Op: OpConfigList}
}

func buildConfigSetRequest(name string, value string) (Request, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Request{}, fmt.Errorf("missing setting name")
	}
	return Request{Op: OpConfigSet, Setting: name, Value: value}, nil
}
