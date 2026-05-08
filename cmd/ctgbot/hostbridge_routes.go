package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	hostbridgetls "github.com/bartdeboer/ctgbot/internal/hostbridge/tls"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerHostbridgeRoutes(r *clir.Router, store *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("hostbridge serve", "Serve the hostbridge server over TCP", func(req *clir.Request) error {
			fs := flag.NewFlagSet("hostbridge serve", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)

			addr := fs.String("addr", getenv("HOSTBRIDGE_ADDR", "127.0.0.1:4567"), "TCP listen address")
			tlsDir := fs.String("tls-dir", "", "Optional TLS material directory containing ca.crt, ca.key, server.crt, server.key")
			timeoutSec := fs.Int("default-timeout-sec", 30, "Default timeout in seconds")
			var allow allowHostbridgeServeFlag
			fs.Var(&allow, "allow", "Additional allowed command mapping in the form name=/absolute/path")

			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(req.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			resolvedTLSDir := strings.TrimSpace(*tlsDir)
			if resolvedTLSDir == "" {
				cfg, err := appstate.NewConfig("", store)
				if err != nil {
					return err
				}
				if err := cfg.EnsurePaths(); err != nil {
					return err
				}
				resolvedTLSDir = cfg.Hostbridge().TLSRoot()
			}

			router, err := commandengine.NewRouter([]commandengine.Definition{schemacommands.RunCommandDefinition()}, commandengine.SourceHostbridge)
			if err != nil {
				return err
			}
			registry := commandengine.NewRegistry()
			if err := hostbridgeserver.RegisterRunCommandHandler(registry, &hostbridgeserver.RunCommandRunner{
				ResolveAllowed:    hostbridgeserver.StaticAllowedCommandResolver(allow.Commands()),
				DefaultTimeoutSec: *timeoutSec,
			}); err != nil {
				return err
			}
			srv := hostbridgeserver.NewCommandServer(commandengine.NewEngine(router, registry))
			srv.Prepare = func(ctx context.Context, clientIdentity string, cmdReq commandengine.Request) (commandengine.Request, error) {
				cmdReq.Context.Source = commandengine.SourceHostbridge
				cmdReq.Context.Actor = commandengine.Actor{ID: strings.TrimSpace(clientIdentity), Roles: []simplerbac.Role{simplerbac.RoleAgent}}
				return cmdReq, nil
			}

			if strings.TrimSpace(resolvedTLSDir) == "" {
				ln, err := hostbridgeserver.Listen(*addr)
				if err != nil {
					return err
				}
				return hostbridgeserver.ServeCommandListener(ctx, ln, srv)
			}

			if err := hostbridgetls.EnsureServerMaterials(resolvedTLSDir); err != nil {
				return err
			}
			tlsConfig, err := hostbridgetls.LoadServerTLSConfig(resolvedTLSDir)
			if err != nil {
				return err
			}
			ln, err := hostbridgeserver.ListenTLS(*addr, tlsConfig)
			if err != nil {
				return err
			}
			return hostbridgeserver.ServeCommandListener(ctx, ln, srv)
		})
	})
}

type allowHostbridgeServeFlag struct {
	values map[string]string
}

func (f *allowHostbridgeServeFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	var parts []string
	for k, v := range f.values {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func (f *allowHostbridgeServeFlag) Set(v string) error {
	if f.values == nil {
		f.values = map[string]string{}
	}
	name, path, ok := strings.Cut(v, "=")
	if !ok {
		return fmt.Errorf("expected name=/absolute/path")
	}
	name = strings.TrimSpace(name)
	path = strings.TrimSpace(path)
	if name == "" || path == "" {
		return fmt.Errorf("expected name=/absolute/path")
	}
	if !filepath.IsAbs(path) {
		return fmt.Errorf("path must be absolute: %s", path)
	}
	f.values[name] = path
	return nil
}

func (f *allowHostbridgeServeFlag) Commands() map[string]hostbridgeserver.AllowedCommand {
	return hostbridgeserver.MergeAllowedCommands(f.values)
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
