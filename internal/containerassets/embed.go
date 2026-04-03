package containerassets

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	_ "embed"
)

//go:embed src.tar.gz
var SrcTarGz []byte

type FileSpec struct {
	Source string
	Target string
}

func BuildContextTar() (io.ReadCloser, error) {
	if len(SrcTarGz) > 0 {
		r, err := gzip.NewReader(bytes.NewReader(SrcTarGz))
		if err != nil {
			return nil, err
		}
		return io.NopCloser(r), nil
	}
	return tarFromModule()
}

func WriteBuildContextGzip(w io.Writer) error {
	rc, err := tarFromModule()
	if err != nil {
		return err
	}
	defer rc.Close()

	gw := gzip.NewWriter(w)
	defer gw.Close()

	_, err = io.Copy(gw, rc)
	return err
}

func SelectedFiles() []FileSpec {
	return []FileSpec{
		{Source: "docker/Dockerfile", Target: "Dockerfile"},
		{Source: "go.mod", Target: "go.mod"},
		{Source: "go.sum", Target: "go.sum"},
		{Source: "cmd/hostbridge/main.go", Target: "cmd/hostbridge/main.go"},
		{Source: "internal/hostbridge/protocol.go", Target: "internal/hostbridge/protocol.go"},
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

func tarFromModule() (io.ReadCloser, error) {
	root, err := moduleRoot()
	if err != nil {
		return nil, err
	}

	buf := &bytes.Buffer{}
	tw := tar.NewWriter(buf)

	for _, spec := range SelectedFiles() {
		src := filepath.Join(root, filepath.FromSlash(spec.Source))
		info, err := os.Stat(src)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			return nil, errors.New("selected file is a directory: " + spec.Source)
		}

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return nil, err
		}
		hdr.Name = filepath.ToSlash(spec.Target)
		hdr.Mode = 0o644

		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}

		f, err := os.Open(src)
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(tw, f); err != nil {
			_ = f.Close()
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}

	return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
}

func SelectedTargetsSummary() string {
	var parts []string
	for _, spec := range SelectedFiles() {
		parts = append(parts, spec.Target)
	}
	return strings.Join(parts, ", ")
}
