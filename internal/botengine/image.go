package botengine

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"

	"github.com/bartdeboer/go-codextgbot/internal/containerassets"
)

type ImageBuilder struct {
	Config *Config
	Logger *log.Logger
}

func (b *ImageBuilder) EnsureImage(ctx context.Context) error {
	if b == nil || b.Config == nil {
		return fmt.Errorf("missing config")
	}
	if imageExists(ctx, b.Config.DockerImage()) {
		return nil
	}
	return b.Build(ctx, false)
}

func (b *ImageBuilder) Build(ctx context.Context, noCache bool) error {
	if b == nil || b.Config == nil {
		return fmt.Errorf("missing config")
	}
	buildDir, err := os.MkdirTemp("", "codextgbot-image-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(buildDir)

	if err := containerassets.WriteBuildContext(buildDir); err != nil {
		return fmt.Errorf("write embedded build context: %w", err)
	}

	args := []string{
		"build",
		"-t", b.Config.DockerImage(),
		"--build-arg", "TARGETARCH=" + runtime.GOARCH,
	}
	if noCache {
		args = append(args, "--no-cache")
	}
	args = append(args, buildDir)

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	b.logf("building docker image=%s build_context=%s", b.Config.DockerImage(), buildDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker build: %w", err)
	}
	return nil
}

func (b *ImageBuilder) logf(format string, args ...any) {
	if b.Logger != nil {
		b.Logger.Printf(format, args...)
	}
}

func imageExists(ctx context.Context, image string) bool {
	cmd := exec.CommandContext(ctx, "docker", "image", "inspect", image)
	return cmd.Run() == nil
}
