package runtimeimage

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/buildassets"
)

const (
	LabelBuildTarget = "ctgbot.build_target"
	LabelBuiltAt     = "ctgbot.built_at"
	LabelGitCommit   = "ctgbot.git_commit"
	LabelHostbridge  = "ctgbot.hostbridge"
	LabelVersion     = "ctgbot.version"
)

type Builder struct {
	Logger    *log.Logger
	SourceDir string
}

// Target describes one ctgbot runtime image build target.
//
// It is not a generic Docker build target. Name and Ref are ctgbot ownership
// metadata used for discovery/status output. Image and Dockerfile are the
// Docker build fields consumed by the standard runtime image builder.
type Target struct {
	Name       string
	Ref        string
	Image      string
	Dockerfile string
	DependsOn  []string
	NoCache    bool
}

func (b *Builder) BuildTarget(ctx context.Context, target Target, noCache bool) error {
	target, err := normalizeTarget(target)
	if err != nil {
		return err
	}
	buildContext, err := buildassets.BuildContextTar()
	if err != nil {
		return err
	}
	defer buildContext.Close()

	args := dockerBuildArgs(target, noCache || target.NoCache, b.buildLabels(ctx, target))

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = buildContext
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	b.logf("building docker image target=%s ref=%s image=%s dockerfile=%s build_context=embedded_tar", target.Name, target.Ref, target.Image, target.Dockerfile)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}
	return nil
}

func dockerBuildArgs(target Target, noCache bool, labels map[string]string) []string {
	args := []string{
		"build",
		"-f", target.Dockerfile,
		"-t", target.Image,
		"--build-arg", "TARGETARCH=" + runtime.GOARCH,
	}
	labelKeys := make([]string, 0, len(labels))
	for key := range labels {
		labelKeys = append(labelKeys, key)
	}
	sort.Strings(labelKeys)
	for _, key := range labelKeys {
		value := strings.TrimSpace(labels[key])
		if value == "" {
			continue
		}
		args = append(args, "--label", key+"="+value)
	}
	if noCache {
		args = append(args, "--no-cache")
	}
	args = append(args, "-")
	return args
}

func normalizeTarget(target Target) (Target, error) {
	target.Name = strings.TrimSpace(target.Name)
	target.Ref = strings.TrimSpace(target.Ref)
	target.Image = strings.TrimSpace(target.Image)
	target.Dockerfile = strings.TrimSpace(target.Dockerfile)
	target.DependsOn = cleanStrings(target.DependsOn)
	if target.Name == "" {
		target.Name = target.Ref
	}
	if target.Name == "" {
		target.Name = target.Image
	}
	if target.Image == "" {
		return Target{}, fmt.Errorf("missing runtime image")
	}
	if target.Dockerfile == "" {
		target.Dockerfile = "Dockerfile"
	}
	return target, nil
}

func cleanStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (b *Builder) buildLabels(ctx context.Context, target Target) map[string]string {
	labels := map[string]string{
		LabelBuildTarget: firstNonEmpty(target.Ref, target.Name, target.Image),
		LabelBuiltAt:     time.Now().UTC().Format(time.RFC3339Nano),
		LabelHostbridge:  "embedded",
		LabelVersion:     buildassets.Version(),
	}
	if commit := CurrentGitCommit(ctx, b.SourceDir); commit != "" {
		labels[LabelGitCommit] = commit
	}
	return labels
}

func CurrentGitCommit(ctx context.Context, dir string) string {
	cmd := exec.CommandContext(ctx, "git", "rev-parse", "--verify", "HEAD")
	if strings.TrimSpace(dir) != "" {
		cmd.Dir = strings.TrimSpace(dir)
	}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (b *Builder) logf(format string, args ...any) {
	if b.Logger != nil {
		b.Logger.Printf(format, args...)
	}
}
