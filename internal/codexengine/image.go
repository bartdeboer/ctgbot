package codexengine

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"runtime"

	"github.com/bartdeboer/go-ctgbot/internal/appconfig"
	"github.com/bartdeboer/go-ctgbot/internal/containerassets"
)

type ImageBuilder struct {
	Config *appconfig.Config
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
	buildContext, err := containerassets.BuildContextTar()
	if err != nil {
		return err
	}
	defer buildContext.Close()

	args := []string{
		"build",
		"-t", b.Config.DockerImage(),
		"--build-arg", "TARGETARCH=" + runtime.GOARCH,
	}
	if noCache {
		args = append(args, "--no-cache")
	}
	args = append(args, "-")

	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = buildContext
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	b.logf("building docker image=%s build_context=embedded_tar", b.Config.DockerImage())
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
