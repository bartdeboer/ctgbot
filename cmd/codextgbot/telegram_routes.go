package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
	"github.com/bartdeboer/go-codextgbot/internal/botengine"
	"github.com/bartdeboer/go-codextgbot/internal/hostbridge"
)

func registerTelegramRoutes(r *clir.Router, store *clistate.Store) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("telegram monitor", "Run the Telegram Codex bot", func(req *clir.Request) error {
			fs := flag.NewFlagSet("telegram monitor", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)

			tokenFlag := fs.String("token", "", "Telegram bot token")
			stateRoot := fs.String("state-root", "", "State root (default: <cwd>/.codextgbot)")
			dbPath := fs.String("db-path", "", "SQLite DB path (default: <state-root>/codextgbot.db)")

			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			token := resolveTelegramToken(*tokenFlag, store)
			if token == "" {
				return fmt.Errorf("missing telegram token (use --token, TELEGRAM_BOT_TOKEN, or codextgbot config --set-telegram-token)")
			}

			logger := log.New(os.Stdout, "", log.LstdFlags)

			cfg, err := botengine.NewConfig(*stateRoot, store)
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

			db, err := botengine.OpenDB(resolvedDBPath, logger)
			if err != nil {
				return err
			}

			api, err := botengine.NewTelegramAPIV2(token)
			if err != nil {
				return err
			}

			storage := botengine.NewConversationStorage(db)
			sessions := &botengine.SessionExecutor{Config: cfg, Logger: logger}
			tb := botengine.NewTelegramBot(api, storage, sessions, cfg, logger)

			if err := tb.AutoMigrate(context.Background()); err != nil {
				return err
			}

			runCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			ln, err := hostbridge.Listen("tcp", cfg.HostbridgeTCPListenAddr())
			if err != nil {
				return fmt.Errorf("start tcphostbridge listener: %w", err)
			}

			bridgeErrCh := make(chan error, 1)
			go func() {
				bridgeErrCh <- hostbridge.ServeListener(runCtx, ln, 30, nil, logger)
			}()

			botErrCh := make(chan error, 1)
			go func() {
				botErrCh <- tb.Run(runCtx)
			}()

			select {
			case err := <-bridgeErrCh:
				stop()
				if err != nil {
					return fmt.Errorf("tcphostbridge runtime: %w", err)
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
