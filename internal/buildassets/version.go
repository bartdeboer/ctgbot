package buildassets

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const BaseVersion = "v0.0.0"

const (
	versionAssetPath = "assets/version.txt"
	devVersionSuffix = "-dev"
)

var semverTagPattern = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+(?:[-+][0-9A-Za-z.-]+)?$`)

type VersionInfo struct {
	Version   string
	Generated bool
}

func Info() VersionInfo {
	return versionInfoFromEmbeddedText(embeddedVersionText())
}

func Version() string {
	return Info().Version
}

func DeriveVersion(ctx context.Context, root string) string {
	return deriveVersionFromGitState(readGitVersionState(ctx, root))
}

func WriteVersionFile(ctx context.Context, root string) (string, error) {
	version := DeriveVersion(ctx, root)
	path := VersionFilePath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(version+"\n"), 0o644); err != nil {
		return "", err
	}
	return version, nil
}

func VersionFilePath(root string) string {
	return filepath.Join(strings.TrimSpace(root), "internal", "buildassets", "assets", "version.txt")
}

func SourceTarGzipPath(root string) string {
	return filepath.Join(strings.TrimSpace(root), "internal", "buildassets", "assets", "src.tar.gz")
}

type gitVersionState struct {
	ExactTag    string
	NearestTag  string
	ShortCommit string
	Dirty       bool
}

func deriveVersionFromGitState(state gitVersionState) string {
	exactTag := cleanSemverTag(state.ExactTag)
	nearestTag := cleanSemverTag(state.NearestTag)
	if exactTag != "" && !state.Dirty {
		return exactTag
	}

	shortCommit := strings.TrimSpace(state.ShortCommit)
	if shortCommit == "" {
		return BaseVersion + devVersionSuffix
	}

	base := firstNonEmptyString(nearestTag, exactTag, BaseVersion)
	version := base + "-" + shortCommit
	if state.Dirty {
		version += devVersionSuffix
	}
	return version
}

func versionInfoFromEmbeddedText(body []byte, err error) VersionInfo {
	fallback := VersionInfo{Version: BaseVersion + devVersionSuffix}
	if err != nil {
		return fallback
	}
	version := strings.TrimSpace(string(body))
	if version == "" {
		return fallback
	}
	return VersionInfo{Version: version, Generated: true}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func cleanSemverTag(tag string) string {
	tag = strings.TrimSpace(tag)
	if !semverTagPattern.MatchString(tag) {
		return ""
	}
	return tag
}

func readGitVersionState(ctx context.Context, root string) gitVersionState {
	state := gitVersionState{
		ExactTag:    gitOutput(ctx, root, "describe", "--tags", "--exact-match", "--match", "v[0-9]*.[0-9]*.[0-9]*", "HEAD"),
		NearestTag:  gitOutput(ctx, root, "describe", "--tags", "--abbrev=0", "--match", "v[0-9]*.[0-9]*.[0-9]*", "HEAD"),
		ShortCommit: gitOutput(ctx, root, "rev-parse", "--short", "HEAD"),
	}
	status := gitOutput(ctx, root, "status", "--porcelain")
	state.Dirty = strings.TrimSpace(status) != ""
	return state
}

func gitOutput(ctx context.Context, root string, args ...string) string {
	cmd := exec.CommandContext(ctx, "git", args...)
	if strings.TrimSpace(root) != "" {
		cmd.Dir = strings.TrimSpace(root)
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func embeddedVersionText() ([]byte, error) {
	body, err := embeddedFiles.ReadFile(versionAssetPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return body, nil
}
