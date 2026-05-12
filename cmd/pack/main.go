package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
)

func main() {
	root, err := moduleRoot()
	if err != nil {
		panic(err)
	}
	if _, err := buildassets.WriteVersionFile(context.Background(), root); err != nil {
		panic(err)
	}
	outPath := buildassets.SourceTarGzipPath(root)
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		panic(err)
	}
	out, err := os.Create(outPath)
	if err != nil {
		panic(err)
	}
	defer out.Close()

	if err := buildassets.WriteBuildContextGzip(out); err != nil {
		panic(err)
	}
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("module root (go.mod) not found")
		}
		dir = parent
	}
}
