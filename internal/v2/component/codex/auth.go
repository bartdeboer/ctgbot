package codex

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/v2/component"
)

const (
	DefaultImage        = "ctgbot-codex:latest"
	DefaultCallbackPort = 1455
	DefaultProfilePath  = "/profile"
)

func (c *Component) Auth(ctx context.Context, req component.AuthRequest) error {
	if req.SandboxManager == nil {
		return fmt.Errorf("missing sandbox manager")
	}
	spec, err := AuthSandboxSpec(req)
	if err != nil {
		return err
	}

	sbx := req.SandboxManager.CreateSandbox(spec)
	if _, err := sbx.Ensure(ctx); err != nil {
		return err
	}

	port := callbackPort(req)
	timeout := callbackTimeout(req)
	relay, err := sbx.OpenHTTPRelayPort(ctx, port, timeout)
	if err != nil {
		return err
	}
	defer relay.Close(context.Background())

	return sbx.Exec(ctx, stdout(req), stderr(req), "codex", "login")
}

func AuthSandboxSpec(req component.AuthRequest) (*sandboxengine.SandboxSpec, error) {
	profileHostPath := strings.TrimSpace(req.ProfileHostPath)
	if profileHostPath == "" {
		return nil, fmt.Errorf("missing profile host path")
	}
	profileContainerPath := strings.TrimSpace(req.ProfileContainerPath)
	if profileContainerPath == "" {
		profileContainerPath = DefaultProfilePath
	}

	return sandboxengine.NewBuilder(authSandboxName(req.ComponentType, req.ProfileName)).
		Image(authImage(req)).
		Workdir(profileContainerPath).
		Env([]string{
			"HOME=" + profileContainerPath,
			"CODEX_HOME=" + profileContainerPath,
		}).
		Mounts([]sandboxengine.Mount{{Source: profileHostPath, Target: profileContainerPath}}).
		SecurityOpts([]string{"seccomp=unconfined"}).
		Cmd([]string{"tail", "-f", "/dev/null"}).
		Build(), nil
}

func authSandboxName(componentType string, profileName string) string {
	componentType = safeName(componentType, ComponentType)
	profileName = safeName(profileName, "default")
	return "ctgbot-auth-" + componentType + "-" + profileName
}

func safeName(value string, fallback string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		value = fallback
	}
	var b strings.Builder
	prevDash := false
	for _, r := range value {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fallback
	}
	return out
}

func authImage(req component.AuthRequest) string {
	if image := strings.TrimSpace(req.Image); image != "" {
		return image
	}
	return DefaultImage
}

func callbackPort(req component.AuthRequest) int {
	if req.CallbackPort > 0 {
		return req.CallbackPort
	}
	return DefaultCallbackPort
}

func callbackTimeout(req component.AuthRequest) time.Duration {
	if req.CallbackTimeout > 0 {
		return req.CallbackTimeout
	}
	return 10 * time.Minute
}

func stdout(req component.AuthRequest) io.Writer {
	if req.Stdout != nil {
		return req.Stdout
	}
	return os.Stdout
}

func stderr(req component.AuthRequest) io.Writer {
	if req.Stderr != nil {
		return req.Stderr
	}
	return os.Stderr
}
