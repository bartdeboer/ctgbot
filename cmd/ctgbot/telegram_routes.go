package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
	"github.com/bartdeboer/ctgbot/internal/appconfig"
	"github.com/bartdeboer/ctgbot/internal/codexengine"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
	"github.com/bartdeboer/ctgbot/internal/hostbridgetls"
	"github.com/bartdeboer/ctgbot/internal/telegramengine"
)

func registerTelegramRoutes(r *clir.Router, store *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("telegram monitor", "Run the Telegram Codex bot", func(req *clir.Request) error {
			fs := flag.NewFlagSet("telegram monitor", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)

			tokenFlag := fs.String("token", "", "Telegram bot token")
			stateRoot := fs.String("state-root", "", "State root (default: <cwd>/.ctgbot)")
			dbPath := fs.String("db-path", "", "SQLite DB path (default: <state-root>/ctgbot.db)")

			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			token := resolveTelegramToken(*tokenFlag, store)
			if token == "" {
				return fmt.Errorf("missing telegram token (use --token, TELEGRAM_BOT_TOKEN, or ctgbot config --set-telegram-token)")
			}

			logger := log.New(os.Stdout, "", log.LstdFlags)

			cfg, err := appconfig.NewConfig(*stateRoot, store)
			if err != nil {
				return err
			}
			if err := cfg.EnsurePaths(); err != nil {
				return err
			}

			resolvedDBPath := strings.TrimSpace(*dbPath)
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
			sessions := codexengine.NewSessionStorage(db)
			executor := &codexengine.SessionExecutor{Config: cfg, Logger: logger}
			tb := telegramengine.NewTelegramBot(api, updates, sessions, executor, cfg, logger)

			if err := tb.AutoMigrate(req.Context()); err != nil {
				return err
			}

			runCtx, stop := signal.NotifyContext(req.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			if err := hostbridgetls.EnsureServerMaterials(cfg.HostbridgeTLSRoot()); err != nil {
				return fmt.Errorf("ensure hostbridge tls server materials: %w", err)
			}
			tlsConfig, err := hostbridgetls.LoadServerTLSConfig(cfg.HostbridgeTLSRoot())
			if err != nil {
				return fmt.Errorf("load hostbridge tls server config: %w", err)
			}

			ln, err := hostbridge.ListenTLS(cfg.HostbridgeTCPListenAddr(), tlsConfig)
			if err != nil {
				return fmt.Errorf("start hostbridge listener: %w", err)
			}

			bridgeErrCh := make(chan error, 1)
			go func() {
				resolveAllowed := func(clientIdentity string) map[string]hostbridge.AllowedCommand {
					chatID, _, ok := cfg.ParseChatContainerName(clientIdentity)
					if !ok {
						return hostbridge.DefaultAllowedCommands()
					}
					return hostbridge.MergeAllowedCommandSpecs(cfg.ChatHostbridgeAllowedCommandSpecs(chatID))
				}
				bridgeErrCh <- hostbridge.ServeListener(runCtx, ln, 30, resolveAllowed, logger)
			}()

			botErrCh := make(chan error, 1)
			go func() {
				botErrCh <- tb.Run(runCtx)
			}()

			select {
			case err := <-bridgeErrCh:
				stop()
				if err != nil {
					return fmt.Errorf("hostbridge runtime: %w", err)
				}
				return nil
			case err := <-botErrCh:
				stop()
				return err
			}
		})
	})
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
