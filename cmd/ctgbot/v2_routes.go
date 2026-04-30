package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	v2component "github.com/bartdeboer/ctgbot/internal/v2/component"
	v2codex "github.com/bartdeboer/ctgbot/internal/v2/component/codex"
	v2gmail "github.com/bartdeboer/ctgbot/internal/v2/component/gmail"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v2/profilemanager"
	"github.com/bartdeboer/ctgbot/internal/v2/repository"
	"github.com/bartdeboer/go-clir"
	"github.com/bartdeboer/go-clistate"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	v2ConfigName = "configv2"
	v2DBName     = "ctgbotv2.db"
)

func registerV2Routes(r *clir.Router) {
	r.Routes(func(b *clir.Builder) {
		b.Handle("run", "Run the experimental v2 ctgbot runtime", func(req *clir.Request) error {
			fs := flag.NewFlagSet("run", flag.ContinueOnError)
			fs.SetOutput(os.Stdout)
			dbPath := fs.String("db-path", "", "v2 SQLite DB path")
			if err := fs.Parse(req.Extra); err != nil {
				return err
			}

			runtime, err := openV2Runtime(req.Context(), v2RuntimeOptions{DBPath: *dbPath})
			if err != nil {
				return err
			}

			fmt.Println("ctgbot v2 runtime initialized")
			fmt.Printf("config: %s\n", runtime.ConfigPath)
			fmt.Printf("database: %s\n", runtime.DBPath)
			fmt.Println("status: event sources are not wired yet")
			return nil
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

			runtime, err := openV2Runtime(req.Context(), v2RuntimeOptions{DBPath: *dbPath, Image: *image})
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

			hostPath, err := runtime.Profiles.Ensure(componentType, profileName)
			if err != nil {
				return err
			}

			if err := runtime.Storage.Components().Save(req.Context(), &coremodel.Component{
				Type:    componentType,
				Enabled: true,
			}); err != nil {
				return err
			}
			if err := runtime.Storage.ComponentProfiles().Save(req.Context(), &coremodel.ComponentProfile{
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
			fmt.Printf("container_path: %s\n", runtime.Profiles.ContainerPath())

			if *prepareOnly {
				fmt.Println("auth: prepare only")
				return nil
			}

			candidate := v2ComponentForType(componentType)
			auth, ok := candidate.(v2component.Authenticator)
			if !ok {
				fmt.Println("auth: not implemented yet")
				return nil
			}

			return auth.Auth(req.Context(), v2component.AuthRequest{
				ComponentType:        componentType,
				ProfileName:          profileName,
				ProfileHostPath:      hostPath,
				ProfileContainerPath: runtime.Profiles.ContainerPath(),
				Image:                runtime.Image,
				CallbackPort:         *callbackPort,
				CallbackTimeout:      *callbackTimeout,
				SandboxManager:       runtime.Sandboxes,
				Stdout:               os.Stdout,
				Stderr:               os.Stderr,
			})
		})
	})
}

type v2Runtime struct {
	ConfigPath string
	DBPath     string
	Image      string
	Config     *clistate.Store
	Storage    repository.Storage
	Profiles   *profilemanager.Manager
	Sandboxes  sandboxengine.Manager
}

type v2RuntimeOptions struct {
	DBPath string
	Image  string
}

func openV2Runtime(ctx context.Context, opts v2RuntimeOptions) (*v2Runtime, error) {
	stateRoot := filepath.Join(".", ".ctgbot")
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		return nil, err
	}

	config, err := clistate.NewCwd("ctgbot", v2ConfigName)
	if err != nil {
		return nil, err
	}
	if err := config.PersistString("version", "v2"); err != nil {
		return nil, err
	}

	resolvedDBPath := strings.TrimSpace(opts.DBPath)
	if resolvedDBPath == "" {
		resolvedDBPath = filepath.Join(stateRoot, v2DBName)
	}
	storage, err := openV2Storage(ctx, resolvedDBPath)
	if err != nil {
		return nil, err
	}

	image := strings.TrimSpace(opts.Image)
	if image == "" {
		image = v2codex.DefaultImage
	}

	return &v2Runtime{
		ConfigPath: filepath.Join(stateRoot, v2ConfigName+".json"),
		DBPath:     resolvedDBPath,
		Image:      image,
		Config:     config,
		Storage:    storage,
		Profiles:   profilemanager.New("."),
		Sandboxes:  sandboxengine.NewSandboxManager(log.New(os.Stdout, "", log.LstdFlags)),
	}, nil
}

func openV2Storage(ctx context.Context, dbPath string) (repository.Storage, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("missing db path")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, err
	}
	db, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	storage := repository.NewGORM(db)
	if err := storage.AutoMigrate(ctx); err != nil {
		return nil, err
	}
	return storage, nil
}

func v2ComponentForType(componentType string) v2component.Component {
	switch strings.ToLower(strings.TrimSpace(componentType)) {
	case v2codex.ComponentType:
		return v2codex.New()
	case v2gmail.ComponentType:
		return v2gmail.New(nil)
	default:
		return nil
	}
}
