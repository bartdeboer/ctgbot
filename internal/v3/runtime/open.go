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
	"github.com/bartdeboer/ctgbot/internal/v3/homes"
	"github.com/bartdeboer/ctgbot/internal/v3/repository"
	"github.com/bartdeboer/ctgbot/internal/v3/workspaces"
	"github.com/bartdeboer/go-clistate"
)

const dbName = "ctgbot-v3.db"

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
		stateRoot = filepath.Join(rootPath, ".ctgbot", "v3")
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

	v3storage := repository.NewGORM(db)
	if err := v3storage.AutoMigrate(ctx); err != nil {
		return nil, err
	}

	auxStorage := gormstorage.New(db)
	if err := auxStorage.AutoMigrate(ctx); err != nil {
		return nil, err
	}

	rt := New(v3storage, nil, homes.New(rootPath))
	rt.Workspaces = workspaces.New(rootPath)
	rt.StateRoot = stateRoot
	rt.DBPath = dbPath
	rt.Config = cfg
	rt.TelegramUpdates = auxStorage.TelegramUpdates()
	rt.Sandboxes = sandboxengine.NewSandboxManager(logger)
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
	if r.Homes == nil {
		return fmt.Errorf("missing component homes")
	}
	return nil
}
