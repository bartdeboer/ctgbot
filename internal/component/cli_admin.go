package component

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type cliAuthCommand struct {
	CallbackPort    int
	CallbackTimeout time.Duration
}

type cliAuthStatusCommand struct{}

type CLIAdminSurface struct {
	component Component
}

var _ Component = (*CLIAdminSurface)(nil)
var _ CommandSurface = (*CLIAdminSurface)(nil)
var _ LocalCommandSurface = (*CLIAdminSurface)(nil)

func NewCLIAdminSurface(component Component) *CLIAdminSurface {
	if component == nil {
		return nil
	}
	if _, ok := component.(Authenticator); ok {
		return &CLIAdminSurface{component: component}
	}
	if _, ok := component.(AuthStatusReporter); ok {
		return &CLIAdminSurface{component: component}
	}
	return nil
}

func (s *CLIAdminSurface) Type() string {
	if s == nil || s.component == nil {
		return ""
	}
	return s.component.Type()
}

func (s *CLIAdminSurface) UsesLocalCommandRoutes() bool { return true }

func (s *CLIAdminSurface) CommandDefinitions() []commandengine.Definition {
	if s == nil || s.component == nil {
		return nil
	}
	definitions := []commandengine.Definition{}
	if _, ok := s.component.(Authenticator); ok {
		definitions = append(definitions, commandengine.Definition{
			Pattern: "auth",
			Help:    "Authenticate this component profile",
			Build:   buildCLIAuthCommand,
			Sources: []commandengine.Source{commandengine.SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
			Aliases: []commandengine.Route{{Pattern: "signin"}},
		})
	}
	if _, ok := s.component.(AuthStatusReporter); ok {
		definitions = append(definitions, commandengine.Definition{
			Pattern: "auth status",
			Help:    "Show authentication status for this component profile",
			Build:   buildCLIAuthStatusCommand,
			Sources: []commandengine.Source{commandengine.SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		})
	}
	return definitions
}

func (s *CLIAdminSurface) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	if auth, ok := s.component.(Authenticator); ok {
		if err := commandengine.RegisterPattern[cliAuthCommand](registry, "auth", func(ctx context.Context, req commandengine.Request, cmd cliAuthCommand) (commandengine.Result, error) {
			_, _ = req, cmd
			if err := auth.Auth(ctx, cmd.CallbackPort, cmd.CallbackTimeout, os.Stdout, os.Stderr); err != nil {
				return commandengine.Result{}, err
			}
			return commandengine.Result{Text: "component auth completed"}, nil
		}); err != nil {
			return err
		}
	}
	if reporter, ok := s.component.(AuthStatusReporter); ok {
		if err := commandengine.RegisterPattern[cliAuthStatusCommand](registry, "auth status", func(ctx context.Context, req commandengine.Request, cmd cliAuthStatusCommand) (commandengine.Result, error) {
			_, _, _ = req, cmd, reporter
			if err := reporter.AuthStatus(ctx, os.Stdout, os.Stderr); err != nil {
				return commandengine.Result{}, err
			}
			return commandengine.Result{}, nil
		}); err != nil {
			return err
		}
	}
	return nil
}

func buildCLIAuthCommand(req *clir.Request) (any, error) {
	fs := flag.NewFlagSet("component auth", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	callbackPort := fs.Int("callback-port", 0, "Optional auth callback relay port")
	callbackTimeout := fs.Duration("callback-timeout", 0, "Optional auth callback relay timeout")
	if err := fs.Parse(req.Extra); err != nil {
		return nil, err
	}
	return cliAuthCommand{
		CallbackPort:    *callbackPort,
		CallbackTimeout: *callbackTimeout,
	}, nil
}

func buildCLIAuthStatusCommand(req *clir.Request) (any, error) {
	if extra := strings.TrimSpace(strings.Join(req.Extra, " ")); extra != "" {
		return nil, fmt.Errorf("unexpected auth status arguments: %s", extra)
	}
	return cliAuthStatusCommand{}, nil
}
