package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bartdeboer/ctgbot/internal/agent/codexengine"
	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/chatbroker"
	"github.com/bartdeboer/ctgbot/internal/chatcommands"
	"github.com/bartdeboer/ctgbot/internal/configcommands"
	"github.com/bartdeboer/ctgbot/internal/configsetters"
	hostbridgeserver "github.com/bartdeboer/ctgbot/internal/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/hostbridgetls"
	"github.com/bartdeboer/ctgbot/internal/messenger/telegramengine"
	"github.com/bartdeboer/ctgbot/internal/policysetter"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
)

func registerTelegramRoutes(r *clir.Router, store *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("telegram monitor", "Run the Telegram Codex bot", func(req *clir.Request) error {
			token, stateRoot, dbPath, err := parseTelegramMonitorOptions(req.Extra, store)
			if err != nil {
				return err
			}

			logger := log.New(os.Stdout, "", log.LstdFlags)
			cfg, err := appstate.NewConfig(stateRoot, store)
			if err != nil {
				return err
			}
			if err := cfg.EnsurePaths(); err != nil {
				return err
			}

			resolvedDBPath := strings.TrimSpace(dbPath)
			if resolvedDBPath == "" {
				resolvedDBPath = cfg.DBPath()
			}

			db, err := codexengine.OpenDB(resolvedDBPath, logger)
			if err != nil {
				return err
			}

			api, err := telegramengine.NewTelegramAPIV2(token)
			if err != nil {
				return err
			}

			updates := telegramengine.NewUpdateStorage(db)
			sessions := chatbroker.NewSessionStorage(db)
			sandboxes := sandboxengine.NewSandboxManager(logger)
			broker := chatbroker.New(cfg, sessions, sandboxes, logger)
			broker.RegisterAgent("codex", codexengine.NewSessionExecutor(cfg, logger))
			bot := telegramengine.NewTelegramBot(api, updates, cfg, logger)
			broker.RegisterInboundChatProvider("telegram", bot)
			broker.RegisterOutboundChatProvider("telegram", bot)

			policyRegistry := policysetter.NewDefaultRegistry(configsetters.NewConfigSetters(cfg, store, nil))
			configService := configcommands.New(policyRegistry)
			broker.ConfigCommands = configService

			runCtx, stop := signal.NotifyContext(req.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			broker.ProcessActions = &runtimeProcessActions{stop: stop, upgrade: func(ctx context.Context) error { return runInstalledCtgbotCommand(ctx, "upgrade") }, logger: logger}

			if err := broker.AutoMigrate(runCtx); err != nil {
				return err
			}
			if err := bot.AutoMigrate(runCtx); err != nil {
				return err
			}

			hostbridgeErrCh := make(chan error, 1)
			go func() { hostbridgeErrCh <- runHostbridgeV2(runCtx, cfg, broker) }()

			botErrCh := make(chan error, 1)
			go func() { botErrCh <- bot.Run(runCtx, broker.HandleInboundPayload) }()

			select {
			case err := <-hostbridgeErrCh:
				stop()
				if errors.Is(err, context.Canceled) {
					return nil
				}
				if err != nil {
					return fmt.Errorf("hostbridge runtime: %w", err)
				}
				return nil
			case err := <-botErrCh:
				stop()
				if errors.Is(err, context.Canceled) {
					return nil
				}
				return err
			}
		})
	})
}

func runHostbridgeV2(ctx context.Context, cfg *appstate.Config, broker *chatbroker.Broker) error {
	if cfg == nil {
		return fmt.Errorf("missing config")
	}
	if err := hostbridgetls.EnsureServerMaterials(cfg.HostbridgeTLSRoot()); err != nil {
		return err
	}
	tlsConfig, err := hostbridgetls.LoadServerTLSConfig(cfg.HostbridgeTLSRoot())
	if err != nil {
		return err
	}
	ln, err := hostbridgeserver.ListenTLS(cfg.HostbridgeTCPListenAddr(), tlsConfig)
	if err != nil {
		return err
	}
	provider := chatbroker.NewChatCommandsProvider(broker)
	srv := hostbridgeserver.NewWithRunnerFactory(func(clientIdentity string) chatcommands.Runner {
		return hostbridgeserver.NewRunnerForClient(cfg.ResolveHostbridgeAllowedCommands, clientIdentity, 30, provider)
	})
	return hostbridgeserver.ServeListener(ctx, ln, srv)
}

func parseTelegramMonitorOptions(args []string, store *clistate.Store) (token string, stateRoot string, dbPath string, err error) {
	fs := flag.NewFlagSet("telegram monitor", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)

	tokenFlag := fs.String("token", "", "Telegram bot token")
	stateRootFlag := fs.String("state-root", "", "State root (default: <cwd>/.ctgbot)")
	dbPathFlag := fs.String("db-path", "", "SQLite DB path (default: <state-root>/ctgbot.db)")

	if err := fs.Parse(args); err != nil {
		return "", "", "", err
	}

	resolvedToken := resolveTelegramToken(*tokenFlag, store)
	if resolvedToken == "" {
		return "", "", "", fmt.Errorf("missing telegram token (use --token, TELEGRAM_BOT_TOKEN, or ctgbot config --set-telegram-token)")
	}

	return resolvedToken, strings.TrimSpace(*stateRootFlag), strings.TrimSpace(*dbPathFlag), nil
}

func resolveTelegramToken(flagVal string, store *clistate.Store) string {
	if strings.TrimSpace(flagVal) != "" {
		return strings.TrimSpace(flagVal)
	}
	if t := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")); t != "" {
		return t
	}
	if store == nil {
		return ""
	}
	return strings.TrimSpace(store.GetString("telegram.token", ""))
}
