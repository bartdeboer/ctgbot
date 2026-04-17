package hostbridge

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/bartdeboer/ctgbot/internal/hostbridgetls"
)

type runtimeConfig interface {
	hostbridgetlsServerTLSRootConfig
	tlsListenerConfig
}

type hostbridgetlsServerTLSRootConfig interface {
	HostbridgeTLSRoot() string
}

type Runtime struct {
	Config            runtimeConfig
	Logger            *log.Logger
	ResolveAllowed    AllowedCommandResolver
	SendFile          SendFileHandler
	SendText          SendTextHandler
	DefaultTimeoutSec int
}

func NewRuntime(cfg runtimeConfig, logger *log.Logger, resolve AllowedCommandResolver, sendFile SendFileHandler, sendText SendTextHandler) *Runtime {
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}
	return &Runtime{
		Config:            cfg,
		Logger:            logger,
		ResolveAllowed:    resolve,
		SendFile:          sendFile,
		SendText:          sendText,
		DefaultTimeoutSec: 30,
	}
}

func (r *Runtime) Run(ctx context.Context) error {
	if r == nil || r.Config == nil {
		return fmt.Errorf("missing config")
	}
	tlsConfig, err := hostbridgetls.InitTLSConfig(r.Config)
	if err != nil {
		return fmt.Errorf("init hostbridge tls config: %w", err)
	}
	ln, err := NewTLSListener(r.Config, tlsConfig)
	if err != nil {
		return fmt.Errorf("start hostbridge listener: %w", err)
	}

	resolveAllowed := r.ResolveAllowed
	if resolveAllowed == nil {
		resolveAllowed = StaticAllowedCommandResolver(nil)
	}
	timeout := r.DefaultTimeoutSec
	if timeout <= 0 {
		timeout = 30
	}
	return ServeListener(ctx, ln, timeout, resolveAllowed, r.SendFile, r.SendText, r.Logger)
}
