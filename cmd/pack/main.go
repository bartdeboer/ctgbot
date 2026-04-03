package main

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/bartdeboer/go-codextgbot/internal/containerassets"
)

func main() {
	root, err := moduleRoot()
	if err != nil {
		panic(err)
	}
	outPath := filepath.Join(root, "internal", "containerassets", "src.tar.gz")
	out, err := os.Create(outPath)
	if err != nil {
		panic(err)
	}
	defer out.Close()

	if err := containerassets.WriteBuildContextGzip(out); err != nil {
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
