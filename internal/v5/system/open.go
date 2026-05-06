package system

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	v5hostbridgeserver "github.com/bartdeboer/ctgbot/internal/v5/hostbridge/server"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5gormstorage "github.com/bartdeboer/ctgbot/internal/v5/repository/gormstorage"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	v5docker "github.com/bartdeboer/ctgbot/internal/v5/runtime/docker"
	v5local "github.com/bartdeboer/ctgbot/internal/v5/runtime/local"
	"github.com/bartdeboer/go-clistate"
	"gorm.io/gorm"
)

const dbName = "ctgbot.db"

type System struct {
	Storage    repository.Storage
	Registry   *component.Registry
	Workspaces map[string]Workspace
	Runtimes   map[string]v5runtime.Factory
	Hostbridge *v5hostbridgeserver.Bridge

	RootDir   string
	StateRoot string
	DBPath    string
	Config    *appstate.Config
	Logger    *log.Logger
	DB        *gorm.DB

	loadedMu sync.RWMutex
	loaded   map[string]*component.Loaded
}

func (s *System) AppConfig() *appstate.Config {
	if s == nil {
		return nil
	}
	return s.Config
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
	db, err := openDB(dbPath, logger)
	if err != nil {
		return nil, err
	}

	storage := v5gormstorage.New(db)
	if err := storage.AutoMigrate(ctx); err != nil {
		return nil, err
	}

	workspaces, err := LoadWorkspaces(rootDir, store)
	if err != nil {
		return nil, err
	}
	runtimes, bridge, err := buildRuntimes(rootDir, stateRoot, cfg, storage, sandboxengine.NewSandboxManager(logger), logger)
	if err != nil {
		return nil, err
	}

	return &System{
		Storage:    storage,
		Workspaces: workspaces,
		Runtimes:   runtimes,
		Hostbridge: bridge,
		RootDir:    rootDir,
		StateRoot:  stateRoot,
		DBPath:     dbPath,
		Config:     cfg,
		Logger:     logger,
		DB:         db,
	}, nil
}

func New(storage repository.Storage, workspaces map[string]Workspace, runtimes map[string]v5runtime.Factory, registry *component.Registry) *System {
	return &System{
		Storage:    storage,
		Workspaces: workspaces,
		Runtimes:   runtimes,
		Registry:   registry,
	}
}

func (s *System) StartHostbridge() (containerAddress string, hostAddress string, err error) {
	if s == nil || s.Hostbridge == nil {
		return "", "", nil
	}
	return s.Hostbridge.Start()
}

func resolveStateRoot(rootDir string, stateRoot string) string {
	stateRoot = strings.TrimSpace(stateRoot)
	if stateRoot == "" {
		return filepath.Join(rootDir, ".ctgbot")
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

func resolveComponentsRoot(stateRoot string) string {
	return filepath.Join(strings.TrimSpace(stateRoot), "components")
}

func buildRuntimes(rootDir string, stateRoot string, cfg *appstate.Config, storage repository.Storage, sandboxes sandboxengine.RuntimeManager, logger *log.Logger) (map[string]v5runtime.Factory, *v5hostbridgeserver.Bridge, error) {
	runtimes := map[string]v5runtime.Factory{}
	listenAddress := strings.TrimSpace(os.Getenv("CTGBOT_HOSTBRIDGE_LISTEN_ADDR"))
	if listenAddress == "" {
		listenAddress = strings.TrimSpace(cfg.Hostbridge().ConfiguredTCPListenAddr())
	}
	if listenAddress == "" {
		listenAddress = v5hostbridgeserver.DefaultListenAddress
	}
	bridge := v5hostbridgeserver.NewBridge(stateRoot, storage, logger).WithListenAddress(listenAddress)
	componentsRoot := resolveComponentsRoot(stateRoot)
	for _, runtimeKind := range []string{"docker", "local"} {
		var runtime v5runtime.Factory
		switch runtimeKind {
		case "docker":
			runtime = v5docker.New(rootDir, componentsRoot, sandboxes, bridge)
		case "local":
			runtime = v5local.New(rootDir, componentsRoot)
		default:
			return nil, nil, fmt.Errorf("unsupported runtime %q", runtimeKind)
		}
		runtimes[runtimeKind] = runtime
	}
	return runtimes, bridge, nil
}
