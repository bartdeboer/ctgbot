package system

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
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
	return gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: gormLogger(logger),
	})
}

func gormLogger(logger *log.Logger) gormlogger.Interface {
	writer := logger
	if writer == nil {
		writer = log.New(io.Discard, "", 0)
	}
	return gormlogger.New(writer, gormlogger.Config{
		SlowThreshold:             200 * time.Millisecond,
		LogLevel:                  gormlogger.Warn,
		IgnoreRecordNotFoundError: true,
		Colorful:                  false,
	})
}
