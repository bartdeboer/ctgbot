package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	v2component "github.com/bartdeboer/ctgbot/internal/v2/component"
	v2brokercomponent "github.com/bartdeboer/ctgbot/internal/v2/component/broker"
	v2codex "github.com/bartdeboer/ctgbot/internal/v2/component/codex"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
	v2runtime "github.com/bartdeboer/ctgbot/internal/v2/runtime"
	"github.com/bartdeboer/go-clir"
)

func registerV2Routes(r *clir.Router) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("run", "Run the experimental v2 ctgbot runtime", func(req *clir.Request) error {
			fs := flag.NewFlagSet("run", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			dbPath := fs.String("db-path", "", "v2 SQLite DB path")
			telegramToken := fs.String("telegram-token", "", "Telegram bot token")
			codexProfile := fs.String("codex-profile", "", "Codex component profile name")
			codexImage := fs.String("codex-image", v2codex.DefaultImage, "Codex runtime image")
			pollTimeout := fs.Duration("telegram-poll-timeout", 30*time.Second, "Telegram long-poll timeout")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rt, err := v2runtime.Open(req.Context(), v2runtime.Options{DBPath: *dbPath, Image: *codexImage})
			if err != nil {
				return err
			}

			fmt.Println("ctgbot v2 runtime initialized")
			fmt.Printf("config: %s\n", rt.ConfigPath)
			fmt.Printf("database: %s\n", rt.DBPath)
			token := v2runtime.ResolveTelegramToken(*telegramToken, rt.Config)
			profileName := v2runtime.ResolveCodexProfile(*codexProfile, rt.Config)
			if token == "" || profileName == "" {
				fmt.Println("status: runtime not started")
				fmt.Println("hint: provide --telegram-token or TELEGRAM_BOT_TOKEN and --codex-profile")
				return nil
			}
			runCtx, stop := context.WithCancel(req.Context())
			defer stop()
			logger := log.New(os.Stdout, "", log.LstdFlags)
			return v2runtime.Run(runCtx, rt, v2runtime.BrokerOptions{
				TelegramToken:           token,
				CodexProfile:            profileName,
				TelegramPollTimeout:     *pollTimeout,
				OperatorTelegramUserIDs: v2runtime.ResolveOperatorTelegramUserIDs(rt.Config),
				Actions: &runtimeProcessActions{
					stop:    stop,
					install: func(ctx context.Context) error { return runInstalledCtgbotCommand(ctx, "install") },
					logger:  logger,
				},
			})
		})

		b.Handle("component auth <component> <profile>", "Prepare a v2 component profile for authentication", func(req *clir.Request) error {
			fs := flag.NewFlagSet("component auth", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			dbPath := fs.String("db-path", "", "v2 SQLite DB path")
			image := fs.String("image", v2codex.DefaultImage, "auth sandbox image")
			prepareOnly := fs.Bool("prepare-only", false, "Only create profile metadata and directories")
			callbackPort := fs.Int("callback-port", v2codex.DefaultCallbackPort, "auth callback relay port")
			callbackTimeout := fs.Duration("callback-timeout", 10*time.Minute, "auth callback relay timeout")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			rt, err := v2runtime.Open(req.Context(), v2runtime.Options{DBPath: *dbPath, Image: *image})
			if err != nil {
				return err
			}

			componentType := strings.TrimSpace(req.Params["component"])
			profileName := strings.TrimSpace(req.Params["profile"])
			if componentType == "" {
				return fmt.Errorf("missing component")
			}
			if profileName == "" {
				return fmt.Errorf("missing profile")
			}

			hostPath, err := rt.Profiles.Ensure(componentType, profileName)
			if err != nil {
				return err
			}

			if err := rt.Storage.Components().Save(req.Context(), &coremodel.Component{
				Type:    componentType,
				Enabled: true,
			}); err != nil {
				return err
			}
			if err := rt.Storage.ComponentProfiles().Save(req.Context(), &coremodel.ComponentProfile{
				ComponentType: componentType,
				ProfileName:   profileName,
				Enabled:       true,
			}); err != nil {
				return err
			}

			fmt.Println("component profile ready")
			fmt.Printf("component: %s\n", componentType)
			fmt.Printf("profile: %s\n", profileName)
			fmt.Printf("host_path: %s\n", hostPath)
			fmt.Printf("container_path: %s\n", rt.Profiles.ContainerPath())

			if *prepareOnly {
				fmt.Println("auth: prepare only")
				return nil
			}

			candidate := v2runtime.ComponentForType(componentType)
			auth, ok := candidate.(v2component.Authenticator)
			if !ok {
				fmt.Println("auth: not implemented yet")
				return nil
			}

			return auth.Auth(req.Context(), v2component.AuthRequest{
				ComponentType:        componentType,
				ProfileName:          profileName,
				ProfileHostPath:      hostPath,
				ProfileContainerPath: rt.Profiles.ContainerPath(),
				Image:                rt.Image,
				CallbackPort:         *callbackPort,
				CallbackTimeout:      *callbackTimeout,
				SandboxManager:       rt.Sandboxes,
				Stdout:               os.Stdout,
				Stderr:               os.Stderr,
			})
		})

		b.Handle("chat", "Run v2 chat administration commands", func(req *clir.Request) error {
			rt, err := v2runtime.Open(req.Context(), v2runtime.Options{})
			if err != nil {
				return err
			}
			brokerComponent := v2brokercomponent.New(rt.Storage, v2brokercomponent.Config{
				CodexProfile: v2runtime.ResolveCodexProfile("", rt.Config),
			})
			engine, err := v2component.NewRegistry(brokerComponent).CommandEngine(commandengine.SourceCLI)
			if err != nil {
				return err
			}
			result, err := engine.Run(req.Context(), commandengine.Request{
				Context: commandengine.Context{
					Source: commandengine.SourceCLI,
					Actor:  commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}},
				},
			}, append([]string{"chat"}, req.Extra...))
			if err != nil {
				return err
			}
			if text := strings.TrimSpace(result.Text); text != "" {
				fmt.Println(text)
			}
			return nil
		})
	})
}
