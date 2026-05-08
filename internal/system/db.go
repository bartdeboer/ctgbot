package system

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openDB(path string, logger *log.Logger) (*gorm.DB, error) {
	if path == "" {
		return nil, fmt.Errorf("missing db path")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("db dir: %w", err)
	}
	if logger != nil {
		logger.Printf("database opened (db=%s)", path)
	}
	return gorm.Open(sqlite.Open(path), &gorm.Config{})
}
