package containerassets

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
)

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
			if err := writeDirectoryToTar(tw, src, spec.Target); err != nil {
				return nil, err
			}
			continue
		}
		if err := writeFileToTar(tw, src, spec.Target, info); err != nil {
			return nil, err
		}
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}

	return io.NopCloser(bytes.NewReader(buf.Bytes())), nil
}

func writeDirectoryToTar(tw *tar.Writer, sourceRoot string, targetRoot string) error {
	return filepath.Walk(sourceRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.ToSlash(filepath.Join(targetRoot, rel))
		if shouldSkipTarget(target) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if info.IsDir() {
			return nil
		}
		return writeFileToTar(tw, path, target, info)
	})
}

func writeFileToTar(tw *tar.Writer, src string, target string, info os.FileInfo) error {
	hdr, err := tar.FileInfoHeader(info, "")
	if err != nil {
		return err
	}
	hdr.Name = filepath.ToSlash(target)
	hdr.Mode = 0o644

	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}

	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(tw, f)
	return err
}

func shouldSkipTarget(target string) bool {
	target = filepath.ToSlash(target)
	switch {
	case strings.HasPrefix(target, ".git/"):
		return true
	case strings.HasPrefix(target, "internal/containerassets/assets/"):
		return true
	default:
		return false
	}
}
