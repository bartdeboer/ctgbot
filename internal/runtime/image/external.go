package runtimeimage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Validate host paths before any Docker command. This is not a sandbox against
// concurrent filesystem changes: the build context must be operator-controlled.
func validateExternalTarget(target Target) (Target, error) {
	if !filepath.IsAbs(target.Context) {
		return Target{}, fmt.Errorf("image context must be an absolute host directory")
	}
	root, err := filepath.EvalSymlinks(target.Context)
	if err != nil {
		return Target{}, fmt.Errorf("image context: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		return Target{}, fmt.Errorf("image context: %w", err)
	}
	if !info.IsDir() {
		return Target{}, fmt.Errorf("image context must be a directory")
	}
	if !filepath.IsLocal(target.Dockerfile) || strings.Contains(target.Dockerfile, "\\") {
		return Target{}, fmt.Errorf("external Dockerfile must be relative and remain within its context")
	}
	file, err := filepath.EvalSymlinks(filepath.Join(root, target.Dockerfile))
	if err != nil {
		return Target{}, fmt.Errorf("external Dockerfile: %w", err)
	}
	rel, err := filepath.Rel(root, file)
	if err != nil || !filepath.IsLocal(rel) {
		return Target{}, fmt.Errorf("external Dockerfile resolves outside its context")
	}
	info, err = os.Stat(file)
	if err != nil {
		return Target{}, fmt.Errorf("external Dockerfile: %w", err)
	}
	if !info.Mode().IsRegular() {
		return Target{}, fmt.Errorf("external Dockerfile must be a regular file")
	}
	target.Context = root
	target.Dockerfile = filepath.Clean(target.Dockerfile)
	return target, nil
}
