package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/bartdeboer/ctgbot/internal/app"
	"github.com/bartdeboer/ctgbot/internal/broker"
	"github.com/bartdeboer/ctgbot/internal/buildassets"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/component/claude"
	"github.com/bartdeboer/ctgbot/internal/component/codex"
	allowlistfilter "github.com/bartdeboer/ctgbot/internal/component/filter/allowlist"
	guardcomponent "github.com/bartdeboer/ctgbot/internal/component/filter/guard"
	"github.com/bartdeboer/ctgbot/internal/component/gmail"
	"github.com/bartdeboer/ctgbot/internal/component/gmailv2"
	heartbeatcomponent "github.com/bartdeboer/ctgbot/internal/component/heartbeat"
	indexingcomponent "github.com/bartdeboer/ctgbot/internal/component/indexing"
	"github.com/bartdeboer/ctgbot/internal/component/llamacpp"
	llamacppagentcomponent "github.com/bartdeboer/ctgbot/internal/component/llamacppagent"
	modelcomponent "github.com/bartdeboer/ctgbot/internal/component/model"
	opscomponent "github.com/bartdeboer/ctgbot/internal/component/ops"
	processcomponent "github.com/bartdeboer/ctgbot/internal/component/process"
	schedulercomponent "github.com/bartdeboer/ctgbot/internal/component/scheduler"
	semanticcomponent "github.com/bartdeboer/ctgbot/internal/component/semantic"
	sqlcomponent "github.com/bartdeboer/ctgbot/internal/component/sql"
	supertoniccomponent "github.com/bartdeboer/ctgbot/internal/component/supertonic"
	"github.com/bartdeboer/ctgbot/internal/component/telegram"
	theatercomponent "github.com/bartdeboer/ctgbot/internal/component/theater"
	whispercppcomponent "github.com/bartdeboer/ctgbot/internal/component/whispercpp"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	nodelistener "github.com/bartdeboer/ctgbot/internal/hostbridge/node"
	"github.com/bartdeboer/ctgbot/internal/identity"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	schedulerpkg "github.com/bartdeboer/ctgbot/internal/scheduler"
	systempkg "github.com/bartdeboer/ctgbot/internal/system"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
	"golang.org/x/sync/errgroup"
)

func registerRuntimeRoutes(r *clir.Router, store *clistate.Store, globalStore *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("run", "Run the ctgbot runtime", func(req *clir.Request) error {
			fs := flag.NewFlagSet("run", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			runCtx, stop := signal.NotifyContext(req.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			rtSystem, err := openSystem(req.Context(), store, newRuntimeProcessActions(globalStore, stop, nil), nil)
			if err != nil {
				return err
			}
			if _, _, err := rtSystem.StartHostbridge(); err != nil {
				return fmt.Errorf("start hostbridge: %w", err)
			}

			fmt.Println("ctgbot runtime initialized")
			fmt.Printf("ctgbot_version: %s\n", buildassets.Version())
			fmt.Printf("state_root: %s\n", rtSystem.StateRoot)
			fmt.Printf("database: %s\n", rtSystem.DBPath)

			logf := func(format string, args ...any) {}
			if rtSystem.Logger != nil {
				logf = rtSystem.Logger.Printf
			}
			appService := app.NewServiceWithLogger(rtSystem.Storage, rtSystem, logf)
			group, groupCtx := errgroup.WithContext(runCtx)
			if remoteAddr := strings.TrimSpace(rtSystem.Config.Hostbridge().RemoteListenAddr()); remoteAddr != "" {
				identityManager := identity.NewManager(filepath.Join(rtSystem.StateRoot, "identity"), "")
				instanceIdentity, err := identityManager.Ensure()
				if err != nil {
					return fmt.Errorf("prepare instance identity: %w", err)
				}
				controllerEngine, err := appService.ControllerCommandEngine(req.Context())
				if err != nil {
					return fmt.Errorf("prepare controller command engine: %w", err)
				}
				listener := &nodelistener.Listener{
					Addr:     remoteAddr,
					Runner:   controllerEngine,
					Storage:  rtSystem.Storage,
					Identity: instanceIdentity,
					Logger:   rtSystem.Logger,
				}
				group.Go(func() error {
					return ignoreRuntimeStop(groupCtx, listener.Run(groupCtx))
				})
			}
			group.Go(func() error {
				return ignoreRuntimeStop(groupCtx, schedulerpkg.New(appService, logf).Run(groupCtx))
			})
			group.Go(func() error {
				return ignoreRuntimeStop(groupCtx, broker.New(appService, logf).Run(groupCtx))
			})
			return group.Wait()
		})
	})
}

func openSystem(ctx context.Context, store *clistate.Store, processActions processcomponent.Actions, logger *log.Logger) (*systempkg.System, error) {
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}
	rtSystem, err := systempkg.Open(ctx, "", "", store, logger)
	if err != nil {
		return nil, err
	}
	rtSystem.Registry, err = newRuntimeRegistry(rtSystem, processActions)
	if err != nil {
		return nil, err
	}
	return rtSystem, nil
}

func newRuntimeRegistry(rtSystem *systempkg.System, processActions processcomponent.Actions) (*component.Registry, error) {
	registry := component.NewRegistry()

	if err := registry.Add(telegram.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return telegram.New(ctx, registration, runtime, home, storage, rtSystem.Logger)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(codex.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return codex.New(ctx, registration, runtime, home, storage, rtSystem.Config, rtSystem.ResolveChatWorkspace, rtSystem.Logger, "")
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(claude.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return claude.New(ctx, registration, runtime, home, storage, rtSystem.ResolveChatWorkspace, rtSystem.Logger)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(gmail.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return gmail.NewWithOptions(ctx, registration, runtime, home, storage, gmail.Options{OAuthClientConfigPath: filepath.Join(rtSystem.StateRoot, "google", "oauth_client.json"), Logger: rtSystem.Logger})
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(gmailv2.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return gmailv2.NewWithOptions(ctx, registration, runtime, home, storage, gmailv2.Options{
			OAuthClientConfigPath: filepath.Join(rtSystem.StateRoot, "google", "oauth_client.json"),
			Logger:                rtSystem.Logger,
			ResolveChatWorkspace:  rtSystem.ResolveChatWorkspace,
		})
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(allowlistfilter.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _ = ctx, runtime, home, registration
		if strings.TrimSpace(registration.Name) != allowlistfilter.Name {
			return nil, fmt.Errorf("unsupported filters component: %s", registration.Ref())
		}
		return allowlistfilter.New(storage), nil
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(guardcomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return guardcomponent.New(ctx, registration, runtime, home, storage, rtSystem, rtSystem.Logger.Printf)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(llamacpp.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return llamacpp.New(ctx, registration, runtime, home, storage, rtSystem, rtSystem.Logger)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(llamacppagentcomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return llamacppagentcomponent.New(ctx, registration, runtime, home, storage, rtSystem, rtSystem.ResolveChatWorkspace, rtSystem.Logger)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(modelcomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return modelcomponent.New(ctx, registration, runtime, home, storage)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(semanticcomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return semanticcomponent.New(ctx, registration, runtime, home, storage, rtSystem, rtSystem.Logger.Printf)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(heartbeatcomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		logf := func(format string, args ...any) {}
		if rtSystem.Logger != nil {
			logf = rtSystem.Logger.Printf
		}
		appService := app.NewServiceWithLogger(storage, rtSystem, logf)
		return heartbeatcomponent.New(ctx, registration, runtime, home, storage, broker.New(appService, logf))
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(indexingcomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return indexingcomponent.New(ctx, registration, runtime, home, storage, rtSystem, rtSystem.Logger.Printf)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(indexingcomponent.SearchType, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return indexingcomponent.NewSearch(ctx, registration, runtime, home, storage, rtSystem, rtSystem.Logger.Printf)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(theatercomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return theatercomponent.New(ctx, registration, runtime, home, storage)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(schedulercomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return schedulercomponent.New(ctx, registration, runtime, home, storage, rtSystem.Logger.Printf)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(opscomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _ = ctx, registration, runtime, home
		logf := func(format string, args ...any) {}
		if rtSystem.Logger != nil {
			logf = rtSystem.Logger.Printf
		}
		return opscomponent.New(app.NewServiceWithLogger(storage, rtSystem, logf), rtSystem.Config), nil
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(whispercppcomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return whispercppcomponent.New(ctx, registration, runtime, home, storage, rtSystem)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(supertoniccomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		return supertoniccomponent.New(ctx, registration, runtime, home, storage, rtSystem)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(sqlcomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _ = ctx, registration, runtime, home
		return sqlcomponent.New(rtSystem.DB)
	}); err != nil {
		return nil, err
	}
	if err := registry.Add(processcomponent.Type, func(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
		_, _, _, _, _ = ctx, registration, runtime, home, storage
		return processcomponent.New(processActions), nil
	}); err != nil {
		return nil, err
	}
	return registry, nil
}

func ignoreRuntimeStop(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, ctx.Err())) {
		return nil
	}
	return err
}
