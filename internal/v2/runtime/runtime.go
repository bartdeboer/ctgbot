// Package runtime assembles the experimental v2 ctgbot runtime.
package runtime

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	v2component "github.com/bartdeboer/ctgbot/internal/v2/component"
	v2codex "github.com/bartdeboer/ctgbot/internal/v2/component/codex"
	v2gmail "github.com/bartdeboer/ctgbot/internal/v2/component/gmail"
	"github.com/bartdeboer/ctgbot/internal/v2/profilemanager"
	"github.com/bartdeboer/ctgbot/internal/v2/repository"
	"github.com/bartdeboer/go-clistate"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

const (
	configName = "configv2"
	dbName     = "ctgbotv2.db"
)

type Runtime struct {
	StateRoot string

	ConfigPath string
	DBPath     string
	Image      string
	Config     *clistate.Store
	Storage    repository.Storage
	Profiles   *profilemanager.Manager
	Sandboxes  sandboxengine.Manager
}

type Options struct {
	DBPath string
	Image  string
}

func Open(ctx context.Context, opts Options) (*Runtime, error) {
	rootPath, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	stateRoot := filepath.Join(rootPath, ".ctgbot")
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		return nil, err
	}

	config, err := clistate.NewCwd("ctgbot", configName)
	if err != nil {
		return nil, err
	}
	if err := config.PersistString("version", "v2"); err != nil {
		return nil, err
	}

	dbPath := resolveDBPath(rootPath, stateRoot, opts.DBPath)
	storage, err := OpenStorage(ctx, dbPath)
	if err != nil {
		return nil, err
	}

	image := strings.TrimSpace(opts.Image)
	if image == "" {
		image = v2codex.DefaultImage
	}

	return &Runtime{
		StateRoot:  stateRoot,
		ConfigPath: filepath.Join(stateRoot, configName+".json"),
		DBPath:     dbPath,
		Image:      image,
		Config:     config,
		Storage:    storage,
		Profiles:   profilemanager.New(rootPath),
		Sandboxes:  sandboxengine.NewSandboxManager(log.New(os.Stdout, "", log.LstdFlags)),
	}, nil
}

func OpenStorage(ctx context.Context, dbPath string) (repository.Storage, error) {
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

func ResolveTelegramToken(flagValue string, config *clistate.Store) string {
	if token := strings.TrimSpace(flagValue); token != "" {
		return token
	}
	if token := strings.TrimSpace(os.Getenv("TELEGRAM_BOT_TOKEN")); token != "" {
		return token
	}
	if config == nil {
		return ""
	}
	return strings.TrimSpace(config.GetString("telegram.token", ""))
}

func ComponentForType(componentType string) v2component.Component {
	switch strings.ToLower(strings.TrimSpace(componentType)) {
	case v2codex.ComponentType:
		return v2codex.New()
	case v2gmail.ComponentType:
		return v2gmail.New(nil)
	default:
		return nil
	}
}

func resolveDBPath(rootPath string, stateRoot string, dbPath string) string {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return filepath.Join(stateRoot, dbName)
	}
	if filepath.IsAbs(dbPath) {
		return filepath.Clean(dbPath)
	}
	return filepath.Clean(filepath.Join(rootPath, dbPath))
}
