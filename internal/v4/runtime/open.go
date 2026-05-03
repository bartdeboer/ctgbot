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
	"github.com/bartdeboer/ctgbot/internal/dbstorage/gormstorage"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/v4/execution"
	"github.com/bartdeboer/ctgbot/internal/v4/homes"
	"github.com/bartdeboer/ctgbot/internal/v4/profiles"
	"github.com/bartdeboer/ctgbot/internal/v4/repository"
	"github.com/bartdeboer/ctgbot/internal/v4/workspaces"
	"github.com/bartdeboer/go-clistate"
)

const dbName = "ctgbot-v4.db"

type OpenOptions struct {
	StateRoot string
	DBPath    string
	Store     *clistate.Store
	Logger    *log.Logger
}

func Open(ctx context.Context, opts OpenOptions) (*Runtime, error) {
	logger := opts.Logger
	if logger == nil {
		logger = log.New(os.Stdout, "", log.LstdFlags)
	}

	rootPath, err := filepath.Abs(".")
	if err != nil {
		return nil, err
	}
	stateRoot := strings.TrimSpace(opts.StateRoot)
	if stateRoot == "" {
		stateRoot = filepath.Join(rootPath, ".ctgbot", "v4")
	} else if !filepath.IsAbs(stateRoot) {
		stateRoot = filepath.Clean(filepath.Join(rootPath, stateRoot))
	}
	if err := os.MkdirAll(stateRoot, 0o755); err != nil {
		return nil, err
	}

	cfg, err := appstate.NewConfig(stateRoot, opts.Store)
	if err != nil {
		return nil, err
	}
	if err := cfg.EnsurePaths(); err != nil {
		return nil, err
	}

	dbPath := strings.TrimSpace(opts.DBPath)
	if dbPath == "" {
		dbPath = filepath.Join(stateRoot, dbName)
	} else if !filepath.IsAbs(dbPath) {
		dbPath = filepath.Clean(filepath.Join(rootPath, dbPath))
	}

	db, err := codexengine.OpenDB(dbPath, logger)
	if err != nil {
		return nil, err
	}

	storage := repository.NewGORM(db)
	if err := storage.AutoMigrate(ctx); err != nil {
		return nil, err
	}

	auxStorage := gormstorage.New(db)
	if err := auxStorage.AutoMigrate(ctx); err != nil {
		return nil, err
	}

	profileManager := profiles.New(rootPath, opts.Store)
	workspaceManager := workspaces.New(rootPath)
	sandboxes := sandboxengine.NewSandboxManager(logger)
	runtimeResolver := execution.NewResolver(execution.CreateRequest{
		Config:     cfg,
		Sandboxes:  sandboxes,
		Workspaces: workspaceManager,
		Logger:     logger,
	}, execution.DockerFactory{}, execution.LocalFactory{})
	rt := New(storage, nil, profileManager, homes.New(profileManager), runtimeResolver)
	rt.Workspaces = workspaceManager
	rt.StateRoot = stateRoot
	rt.DBPath = dbPath
	rt.Config = cfg
	rt.TelegramUpdates = auxStorage.TelegramUpdates()
	rt.Sandboxes = sandboxes
	rt.Logger = logger
	rt.DB = db
	return rt, nil
}

func (r *Runtime) RequireReady() error {
	if r == nil {
		return fmt.Errorf("missing runtime")
	}
	if r.Storage == nil {
		return fmt.Errorf("missing runtime storage")
	}
	if r.Profiles == nil {
		return fmt.Errorf("missing profile manager")
	}
	if r.Homes == nil {
		return fmt.Errorf("missing component homes")
	}
	if r.Runtimes == nil {
		return fmt.Errorf("missing runtime resolver")
	}
	return nil
}
