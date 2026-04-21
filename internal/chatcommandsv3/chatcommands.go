package chatcommandsv3

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	clir "github.com/bartdeboer/go-clir"
)

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
			return c.execute(buildRunCommand(req.Params["command"], req.Extra, stdinData))
		})

		b.Handle("sendfile <path>", "Upload a file", func(req *clir.Request) error {
			fs := flag.NewFlagSet("chatcommands sendfile", flag.ContinueOnError)
			fs.SetOutput(io.Discard)
			caption := fs.String("caption", "", "Optional caption")
			contentType := fs.String("type", "", "Optional content type")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}
			command, err := buildSendFile(req.Params["path"], *caption, *contentType)
			if err != nil {
				return err
			}
			return c.execute(command)
		})

		b.Handle("config list", "List config", func(req *clir.Request) error {
			return c.execute(buildConfigList())
		})

		b.Handle("config set <name> <value>", "Set config", func(req *clir.Request) error {
			command, err := buildConfigSet(req.Params["name"], req.Params["value"])
			if err != nil {
				return err
			}
			return c.execute(command)
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
			return c.execute(buildSendText(string(stdinData), strings.TrimSpace(*contentType), *fenced, *language, *syntax))
		})
	})
	return r
}

func (c *ChatCommands) execute(command Command) error {
	if c == nil || c.runner == nil {
		return fmt.Errorf("chat command runner is unavailable")
	}
	return c.runner.Execute(command)
}
