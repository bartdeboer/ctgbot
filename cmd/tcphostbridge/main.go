package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-codextgbot/internal/hostbridge"
	"github.com/bartdeboer/go-codextgbot/internal/hostbridgetls"
)

func main() {
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "help" {
		printHelp()
		return
	}
	if len(args) == 0 {
		args = []string{"serve"}
	}

	r := clir.New()
	r.Routes(func(b *clir.Builder) {
		b.Handle("serve", "Serve the hostbridge controller over TCP", func(req *clir.Request) error {
			fs := flag.NewFlagSet("tcphostbridge serve", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)

			addr := fs.String("addr", getenv("HOSTBRIDGE_ADDR", "127.0.0.1:4567"), "TCP listen address")
			tlsDir := fs.String("tls-dir", "", "Optional TLS material directory containing ca.crt, ca.key, server.crt, server.key")
			timeoutSec := fs.Int("default-timeout-sec", 30, "Default timeout in seconds")
			var allow allowFlag
			fs.Var(&allow, "allow", "Additional allowed command mapping in the form name=/absolute/path")

			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			logger := log.New(os.Stdout, "", log.LstdFlags)
			if strings.TrimSpace(*tlsDir) == "" {
				return hostbridge.Serve(ctx, *addr, *timeoutSec, allow.Commands(), logger)
			}

			if err := hostbridgetls.EnsureServerMaterials(*tlsDir); err != nil {
				return err
			}
			tlsConfig, err := hostbridgetls.LoadServerTLSConfig(*tlsDir)
			if err != nil {
				return err
			}
			ln, err := hostbridge.ListenTLS(*addr, tlsConfig)
			if err != nil {
				return err
			}
			return hostbridge.ServeListener(ctx, ln, *timeoutSec, hostbridge.StaticAllowedCommandResolver(allow.Commands()), logger)
		})
	})

	if err := r.Run(context.Background(), args); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		printHelp()
		os.Exit(1)
	}
}

type allowFlag struct {
	values map[string]string
}

func (f *allowFlag) String() string {
	if len(f.values) == 0 {
		return ""
	}
	var parts []string
	for k, v := range f.values {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, ",")
}

func (f *allowFlag) Set(v string) error {
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

func (f *allowFlag) Commands() map[string]hostbridge.AllowedCommand {
	return hostbridge.MergeAllowedCommands(f.values)
}

func getenv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func printHelp() {
	fmt.Fprintln(os.Stdout, "usage: tcphostbridge serve [--addr 127.0.0.1:PORT] [--tls-dir DIR] [--allow name=/absolute/path]")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "note: hostbridge enforces loopback-only binds and rejects non-127.0.0.1 addresses.")
	fmt.Fprintln(os.Stdout, "")
	fmt.Fprintln(os.Stdout, "examples:")
	fmt.Fprintln(os.Stdout, "  tcphostbridge serve")
	fmt.Fprintln(os.Stdout, "  tcphostbridge serve --tls-dir ./.codextgbot/tls")
	fmt.Fprintln(os.Stdout, "  tcphostbridge serve --addr 127.0.0.1:4567 --allow ls=/bin/ls")
}
