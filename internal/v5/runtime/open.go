package runtime

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/agent/codexengine"
	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	"github.com/bartdeboer/go-clistate"
	"gorm.io/gorm"
)

const dbName = "ctgbot-v5.db"

type System struct {
	Storage  repository.Storage
	Registry *component.Registry
	Profiles map[string]component.Profile
	Runtimes map[string]component.Runtime

	RootDir   string
	StateRoot string
	DBPath    string
	Config    *appstate.Config
	Logger    *log.Logger
	DB        *gorm.DB

	loaded map[string]*component.Loaded
}

func Open(ctx context.Context, stateRoot string, dbPath string, store *clistate.Store, logger *log.Logger) (*System, error) {
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}

	rootDir, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	stateRoot = resolveStateRoot(rootDir, stateRoot)
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		return nil, err
	}

	cfg, err := appstate.NewConfig(stateRoot, store)
	if err != nil {
		return nil, err
	}
	if err := cfg.EnsurePaths(); err != nil {
		return nil, err
	}

	dbPath = resolveDBPath(rootDir, stateRoot, dbPath)
	db, err := codexengine.OpenDB(dbPath, logger)
	if err != nil {
		return nil, err
	}

	storage := repository.NewGORM(db)
	if err := storage.AutoMigrate(ctx); err != nil {
		return nil, err
	}

	profiles, err := LoadProfiles(rootDir, store)
	if err != nil {
		return nil, err
	}
	runtimes, err := buildRuntimes(rootDir, sandboxengine.NewSandboxManager(logger), profiles)
	if err != nil {
		return nil, err
	}

	return &System{
		Storage:   storage,
		Profiles:  profiles,
		Runtimes:  runtimes,
		RootDir:   rootDir,
		StateRoot: stateRoot,
		DBPath:    dbPath,
		Config:    cfg,
		Logger:    logger,
		DB:        db,
	}, nil
}

func New(storage repository.Storage, profiles map[string]component.Profile, runtimes map[string]component.Runtime, registry *component.Registry) *System {
	return &System{
		Storage:  storage,
		Profiles: profiles,
		Runtimes: runtimes,
		Registry: registry,
	}
}

func resolveStateRoot(rootDir string, stateRoot string) string {
	stateRoot = strings.TrimSpace(stateRoot)
	if stateRoot == "" {
		return filepath.Join(rootDir, ".ctgbot", "v5")
	}
	if filepath.IsAbs(stateRoot) {
		return filepath.Clean(stateRoot)
	}
	return filepath.Clean(filepath.Join(rootDir, stateRoot))
}

func resolveDBPath(rootDir string, stateRoot string, dbPath string) string {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return filepath.Join(stateRoot, dbName)
	}
	if filepath.IsAbs(dbPath) {
		return filepath.Clean(dbPath)
	}
	return filepath.Clean(filepath.Join(rootDir, dbPath))
}

func buildRuntimes(rootDir string, sandboxes sandboxengine.RuntimeManager, profiles map[string]component.Profile) (map[string]component.Runtime, error) {
	runtimes := map[string]component.Runtime{}
	for name, profile := range profiles {
		var runtime component.Runtime
		switch profile.Runtime {
		case "docker":
			runtime = newDockerRuntime(rootDir, sandboxes, profile)
		case "local":
			runtime = newLocalRuntime(rootDir, profile)
		default:
			return nil, fmt.Errorf("unsupported runtime %q for profile %s", profile.Runtime, name)
		}
		runtimes[name] = runtime
	}
	return runtimes, nil
}
