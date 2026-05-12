package buildassets

import (
	"embed"
	"errors"
	"io/fs"
)

//go:embed assets/*
var embeddedFiles embed.FS

func embeddedSrcTarGz() ([]byte, error) {
	body, err := fs.ReadFile(embeddedFiles, "assets/src.tar.gz")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if len(body) == 0 {
		return nil, nil
	}
	return body, nil
}
