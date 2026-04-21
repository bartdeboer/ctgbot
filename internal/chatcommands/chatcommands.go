package chatcommands

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

type parseState struct {
	Request Request
}

type parseStateKey struct{}

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

func (c *ChatCommands) Parse(ctx context.Context, base Request, argv []string) (Request, error) {
	if c == nil || c.router == nil {
		return Request{}, fmt.Errorf("chat commands are unavailable")
	}
	state := &parseState{Request: base}
	runCtx := context.WithValue(ctx, parseStateKey{}, state)
	if err := c.router.Run(runCtx, argv); err != nil {
		return Request{}, err
	}
	if state.Request.Command == nil {
		return Request{}, fmt.Errorf("missing command")
	}
	return state.Request, nil
}

func (c *ChatCommands) Execute(ctx context.Context, req Request) (Result, error) {
	if c == nil || c.runner == nil {
		return Result{}, fmt.Errorf("chat command runner is unavailable")
	}
	if req.Command == nil {
		return Result{}, fmt.Errorf("missing command")
	}
	return c.runner.Execute(ctx, req)
}

func (c *ChatCommands) Run(ctx context.Context, argv []string) (Result, error) {
	return c.RunRequest(ctx, Request{}, argv)
}

func (c *ChatCommands) RunRequest(ctx context.Context, base Request, argv []string) (Result, error) {
	req, err := c.Parse(ctx, base, argv)
	if err != nil {
		return Result{}, err
	}
	return c.Execute(ctx, req)
}

func (c *ChatCommands) newRouter() *clir.Router {
	r := clir.New()
	r.Routes(func(b *clir.Builder) {
		b.Handle("run <command>", "Run a whitelisted host command", func(req *clir.Request) error {
			stdinData, err := io.ReadAll(os.Stdin)
			if err != nil {
				return fmt.Errorf("read stdin: %w", err)
			}
			return setParsedCommand(req, buildRunCommand(req.Params["command"], req.Extra, stdinData))
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
			return setParsedCommand(req, command)
		})

		b.Handle("config list", "List config", func(req *clir.Request) error {
			return setParsedCommand(req, buildConfigList())
		})

		b.Handle("config set <name> <value>", "Set config", func(req *clir.Request) error {
			command, err := buildConfigSet(req.Params["name"], req.Params["value"])
			if err != nil {
				return err
			}
			return setParsedCommand(req, command)
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
			command := buildSendText(string(stdinData), strings.TrimSpace(*contentType), *fenced, *language, *syntax)
			return setParsedCommand(req, command)
		})

		b.Handle("refresh", "Refresh the active conversation container", func(req *clir.Request) error {
			return setParsedCommand(req, buildRefreshContainer())
		})
		b.Handle("container refresh", "Refresh the active conversation container", func(req *clir.Request) error {
			return setParsedCommand(req, buildRefreshContainer())
		})

		b.Handle("purge", "Purge the active conversation chat state", func(req *clir.Request) error {
			return setParsedCommand(req, buildPurgeChat())
		})
		b.Handle("chat purge", "Purge the active conversation chat state", func(req *clir.Request) error {
			return setParsedCommand(req, buildPurgeChat())
		})

		b.Handle("interrupt", "Interrupt the active turn", func(req *clir.Request) error {
			return setParsedCommand(req, buildInterruptTurn())
		})
		b.Handle("upgrade", "Upgrade ctgbot in-place", func(req *clir.Request) error {
			return setParsedCommand(req, buildUpgrade())
		})
		b.Handle("quit", "Quit ctgbot", func(req *clir.Request) error {
			return setParsedCommand(req, buildQuit())
		})
	})
	return r
}

func setParsedCommand(req *clir.Request, command Command) error {
	if req == nil {
		return fmt.Errorf("missing request")
	}
	state, ok := req.Context().Value(parseStateKey{}).(*parseState)
	if !ok || state == nil {
		return fmt.Errorf("missing parse state")
	}
	state.Request.Command = command
	return nil
}
