package remote

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	gobtransport "github.com/bartdeboer/ctgbot/internal/hostbridge/transport/gob"
	httptransport "github.com/bartdeboer/ctgbot/internal/hostbridge/transport/http"
	"github.com/bartdeboer/ctgbot/internal/identity"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

const Type = "remote"
const CommandsPath = "/commands"

type ControllerRuntime interface {
	ControllerCommandEngine(ctx context.Context) (*commandengine.Engine, error)
	InstanceIdentity(ctx context.Context) (identity.Identity, error)
}

type Component struct {
	Runtime ControllerRuntime
}

type RunCommand struct {
	URL  string
	Args []string
}

var _ component.CommandSurface = (*Component)(nil)

func New(runtime ControllerRuntime) *Component {
	return &Component{Runtime: runtime}
}

func RegisterGobTypes(register func(any)) {
	register(RunCommand{})
}

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{{
		Pattern:               "remote run <url>",
		Help:                  "Run a typed command on a trusted remote ctgbot node",
		Build:                 buildRunCommand,
		Sources:               []commandengine.Source{commandengine.SourceCLI, commandengine.SourceHostbridge},
		Policy:                simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		InstructionVisibility: commandengine.InstructionImportant,
	}}
}

func buildRunCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("remote run", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	args := fs.Args()
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) == 0 {
		return nil, fmt.Errorf("missing remote command args")
	}
	url := strings.TrimSpace(req.Params["url"])
	if url == "" {
		return nil, fmt.Errorf("missing remote node URL")
	}
	return RunCommand{URL: url, Args: append([]string(nil), args...)}, nil
}

func (c *Component) RegisterCommandHandlers(registry *commandengine.Registry) error {
	return commandengine.Register[RunCommand](registry, c.handleRun)
}

func (c *Component) handleRun(ctx context.Context, req commandengine.Request, cmd RunCommand) (commandengine.Result, error) {
	_ = req
	if c == nil || c.Runtime == nil {
		return commandengine.Result{}, fmt.Errorf("missing remote runtime")
	}
	self, err := c.Runtime.InstanceIdentity(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	engine, err := c.Runtime.ControllerCommandEngine(ctx)
	if err != nil {
		return commandengine.Result{}, err
	}
	remoteReq, err := engine.Parse(ctx, commandengine.Request{Context: commandengine.Context{
		Source: commandengine.SourceController,
		Actor: commandengine.Actor{
			ID:    self.Fingerprint,
			Label: self.DisplayName,
			Roles: []simplerbac.Role{simplerbac.RoleController},
		},
	}}, cmd.Args)
	if err != nil {
		return commandengine.Result{}, fmt.Errorf("parse remote command: %w", err)
	}
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: identity.ClientTLSConfig(self, true).Clone()}}
	runner := &gobtransport.CommandRunner{Transport: &httptransport.ByteTransport{
		URL:    commandsURL(cmd.URL),
		Client: client,
	}}
	resp, err := runner.RunCommand(ctx, hostbridge.CommandRequest{Request: remoteReq})
	if err != nil {
		return commandengine.Result{}, err
	}
	return resp.Result, nil
}

func commandsURL(raw string) string {
	base := strings.TrimRight(strings.TrimSpace(raw), "/")
	if base == "" {
		return ""
	}
	if strings.HasSuffix(base, CommandsPath) {
		return base
	}
	return base + CommandsPath
}
