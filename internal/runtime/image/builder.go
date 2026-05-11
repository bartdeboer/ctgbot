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

	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/runtime/imageassets"
)

const (
	LabelBuildTarget = "ctgbot.build_target"
	LabelBuiltAt     = "ctgbot.built_at"
	LabelGitCommit   = "ctgbot.git_commit"
	LabelHostbridge  = "ctgbot.hostbridge"
)

type Builder struct {
	Config    *appstate.Config
	Logger    *log.Logger
	SourceDir string
}

// Target describes one buildable Docker runtime image.
//
// Ref identifies the owning component instance when there is one. Name
// identifies the build target when a component owns more than one image.
type Target struct {
	Name       string
	Ref        string
	Image      string
	Dockerfile string
}

func (b *Builder) EnsureImage(ctx context.Context) error {
	if b == nil || b.Config == nil {
		return fmt.Errorf("missing config")
	}
	return b.EnsureTarget(ctx, DefaultTarget(b.Config))
}

func (b *Builder) EnsureTarget(ctx context.Context, target Target) error {
	target, err := normalizeTarget(target)
	if err != nil {
		return err
	}
	if imageExists(ctx, target.Image) {
		return nil
	}
	return b.BuildTarget(ctx, target, false)
}

func (b *Builder) Build(ctx context.Context, noCache bool) error {
	if b == nil || b.Config == nil {
		return fmt.Errorf("missing config")
	}
	return b.BuildTarget(ctx, DefaultTarget(b.Config), noCache)
}

func (b *Builder) BuildTarget(ctx context.Context, target Target, noCache bool) error {
	target, err := normalizeTarget(target)
	if err != nil {
		return err
	}
	buildContext, err := imageassets.BuildContextTar()
	if err != nil {
		return err
	}
	defer buildContext.Close()

	args := dockerBuildArgs(target, noCache, b.buildLabels(ctx, target))

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

func DefaultTarget(cfg *appstate.Config) Target {
	if cfg == nil {
		return Target{}
	}
	return Target{
		Name:       "codex",
		Ref:        "codex",
		Image:      strings.TrimSpace(cfg.Docker().Image()),
		Dockerfile: strings.TrimSpace(cfg.Docker().Dockerfile()),
	}
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

func (b *Builder) buildLabels(ctx context.Context, target Target) map[string]string {
	labels := map[string]string{
		LabelBuildTarget: firstNonEmpty(target.Ref, target.Name, target.Image),
		LabelBuiltAt:     time.Now().UTC().Format(time.RFC3339Nano),
		LabelHostbridge:  "embedded",
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

func imageExists(ctx context.Context, image string) bool {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	return cmd.Run() == nil
}
