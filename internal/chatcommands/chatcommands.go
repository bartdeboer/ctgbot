package chatcommands

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	clir "github.com/bartdeboer/go-clir"
)

type ChatCommands struct {
	user   *clir.Router
	bridge *clir.Router
	runner Runner
}

type parseState struct {
	Request Request
}

type parseStateKey struct{}

type routeName string

type routeSpec struct {
	Pattern string
	Desc    string
	Handler func(req *clir.Request) error
}

const (
	routeRunCommand       routeName = "run-command"
	routeSendFile         routeName = "send-file"
	routeSendStdin        routeName = "send-stdin"
	routeConfigList       routeName = "config-list"
	routeConfigSet        routeName = "config-set"
	routeRefresh          routeName = "refresh"
	routeContainerRefresh routeName = "container-refresh"
	routePurge            routeName = "purge"
	routeChatPurge        routeName = "chat-purge"
	routeInterrupt        routeName = "interrupt"
	routeUpgrade          routeName = "upgrade"
	routeQuit             routeName = "quit"
)

var userRoutes = []routeName{
	routeConfigList,
	routeConfigSet,
	routeRefresh,
	routeContainerRefresh,
	routePurge,
	routeChatPurge,
	routeInterrupt,
	routeUpgrade,
	routeQuit,
}

var bridgeRoutes = []routeName{
	routeRunCommand,
	routeSendFile,
	routeSendStdin,
	routeConfigList,
	routeConfigSet,
	routeRefresh,
	routeContainerRefresh,
	routePurge,
	routeChatPurge,
	routeInterrupt,
	routeUpgrade,
	routeQuit,
}

func New(runner Runner) *ChatCommands {
	c := &ChatCommands{runner: runner}
	c.user = c.newRouter(userRoutes)
	c.bridge = c.newRouter(bridgeRoutes)
	return c
}

func (c *ChatCommands) UserRouter() *clir.Router {
	if c == nil {
		return nil
	}
	return c.user
}

func (c *ChatCommands) BridgeRouter() *clir.Router {
	if c == nil {
		return nil
	}
	return c.bridge
}

func (c *ChatCommands) ParseUser(ctx context.Context, base Request, argv []string) (Request, error) {
	return c.parseWithRouter(ctx, c.UserRouter(), base, argv)
}

func (c *ChatCommands) ParseBridge(ctx context.Context, base Request, argv []string) (Request, error) {
	return c.parseWithRouter(ctx, c.BridgeRouter(), base, argv)
}

func (c *ChatCommands) Parse(ctx context.Context, base Request, argv []string) (Request, error) {
	return c.ParseBridge(ctx, base, argv)
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

func (c *ChatCommands) RunUserRequest(ctx context.Context, base Request, argv []string) (Result, error) {
	req, err := c.ParseUser(ctx, base, argv)
	if err != nil {
		return Result{}, err
	}
	return c.Execute(ctx, req)
}

func (c *ChatCommands) RunBridgeRequest(ctx context.Context, base Request, argv []string) (Result, error) {
	req, err := c.ParseBridge(ctx, base, argv)
	if err != nil {
		return Result{}, err
	}
	return c.Execute(ctx, req)
}

func (c *ChatCommands) Run(ctx context.Context, argv []string) (Result, error) {
	return c.RunBridgeRequest(ctx, Request{}, argv)
}

func (c *ChatCommands) RunRequest(ctx context.Context, base Request, argv []string) (Result, error) {
	return c.RunBridgeRequest(ctx, base, argv)
}

func (c *ChatCommands) UserHelpText() string {
	return formatHelp(c.UserRouter(), true)
}

func (c *ChatCommands) BridgeHelpText() string {
	return formatHelp(c.BridgeRouter(), false)
}

func (c *ChatCommands) parseWithRouter(ctx context.Context, router *clir.Router, base Request, argv []string) (Request, error) {
	if c == nil || router == nil {
		return Request{}, fmt.Errorf("chat commands are unavailable")
	}
	state := &parseState{Request: base}
	runCtx := context.WithValue(ctx, parseStateKey{}, state)
	if err := router.Run(runCtx, argv); err != nil {
		return Request{}, err
	}
	if state.Request.Command == nil {
		return Request{}, fmt.Errorf("missing command")
	}
	return state.Request, nil
}

func (c *ChatCommands) newRouter(names []routeName) *clir.Router {
	r := clir.New()
	r.Routes(func(b *clir.Builder) {
		for _, name := range names {
			spec := c.routeSpecs()[name]
			b.Handle(spec.Pattern, spec.Desc, spec.Handler)
		}
	})
	return r
}

func (c *ChatCommands) routeSpecs() map[routeName]routeSpec {
	return map[routeName]routeSpec{
		routeRunCommand: {
			Pattern: "run <command>",
			Desc:    "Run a whitelisted host command",
			Handler: func(req *clir.Request) error {
				stdinData, err := io.ReadAll(os.Stdin)
				if err != nil {
					return fmt.Errorf("read stdin: %w", err)
				}
				return setParsedCommand(req, buildRunCommand(req.Params["command"], req.Extra, stdinData))
			},
		},
		routeSendFile: {
			Pattern: "sendfile <path>",
			Desc:    "Upload a file",
			Handler: func(req *clir.Request) error {
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
			},
		},
		routeSendStdin: {
			Pattern: "sendstdin",
			Desc:    "Send stdin as text",
			Handler: func(req *clir.Request) error {
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
			},
		},
		routeConfigList: {
			Pattern: "config list",
			Desc:    "List config",
			Handler: func(req *clir.Request) error { return setParsedCommand(req, buildConfigList()) },
		},
		routeConfigSet: {
			Pattern: "config set <name> <value>",
			Desc:    "Set config",
			Handler: func(req *clir.Request) error {
				command, err := buildConfigSet(req.Params["name"], req.Params["value"])
				if err != nil {
					return err
				}
				return setParsedCommand(req, command)
			},
		},
		routeRefresh: {
			Pattern: "refresh",
			Desc:    "Refresh the active container",
			Handler: func(req *clir.Request) error { return setParsedCommand(req, buildRefreshContainer()) },
		},
		routeContainerRefresh: {
			Pattern: "container refresh",
			Desc:    "Refresh the active container",
			Handler: func(req *clir.Request) error { return setParsedCommand(req, buildRefreshContainer()) },
		},
		routePurge: {
			Pattern: "purge",
			Desc:    "Purge the active chat state",
			Handler: func(req *clir.Request) error { return setParsedCommand(req, buildPurgeChat()) },
		},
		routeChatPurge: {
			Pattern: "chat purge",
			Desc:    "Purge the active chat state",
			Handler: func(req *clir.Request) error { return setParsedCommand(req, buildPurgeChat()) },
		},
		routeInterrupt: {
			Pattern: "interrupt",
			Desc:    "Interrupt the active turn",
			Handler: func(req *clir.Request) error { return setParsedCommand(req, buildInterruptTurn()) },
		},
		routeUpgrade: {
			Pattern: "upgrade",
			Desc:    "Upgrade ctgbot",
			Handler: func(req *clir.Request) error { return setParsedCommand(req, buildUpgrade()) },
		},
		routeQuit: {
			Pattern: "quit",
			Desc:    "Quit ctgbot",
			Handler: func(req *clir.Request) error { return setParsedCommand(req, buildQuit()) },
		},
	}
}

func formatHelp(router *clir.Router, slash bool) string {
	if router == nil {
		return ""
	}
	var buf bytes.Buffer
	router.PrintHelp(&buf)
	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	for i, line := range lines {
		if slash && strings.HasPrefix(line, "  ") {
			lines[i] = "  /" + strings.TrimPrefix(line, "  ")
		}
	}
	return "Commands:\n" + strings.Join(lines, "\n")
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
