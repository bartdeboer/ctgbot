package codexengine

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/appstate"
)

type CodexManager struct {
	Config *appstate.Config
	Logger *log.Logger
}

func (m *CodexManager) SignIn(ctx context.Context, deviceAuth bool, withAPIKey bool) error {
	if m == nil || m.Config == nil {
		return fmt.Errorf("missing config")
	}
	if err := m.Config.EnsureCodexCLIHome(); err != nil {
		return err
	}

	builder := &ImageBuilder{Config: m.Config, Logger: m.Logger}
	if err := builder.EnsureImage(ctx); err != nil {
		return err
	}

	relay, err := startSigninRelay(m.Config.DockerCLIContainerName(), appstate.CodexLoginCallbackPort, m.Logger)
	if err != nil {
		return err
	}
	defer relay.Close(context.Background())

	args := []string{
		"run",
		"--rm",
		"-i",
		"--security-opt", "seccomp=unconfined",
		"--name", m.Config.DockerCLIContainerName(),
		"--env", "HOME=" + m.Config.ContainerHomePath(),
		"--env", "CODEX_HOME=" + m.Config.ContainerHomePath(),
		"--workdir", m.Config.ContainerHomePath(),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", m.Config.CodexCLIHomeRoot(), m.Config.ContainerHomePath()),
		m.Config.DockerImage(),
		"codex",
		"login",
	}
	if deviceAuth {
		args = append(args, "--device-auth")
	}
	if withAPIKey {
		args = append(args, "--with-api-key")
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	m.logf("starting containerized codex signin codex_home=%s callback_port=%d", m.Config.CodexCLIHomeRoot(), appstate.CodexLoginCallbackPort)
	return cmd.Run()
}

func (m *CodexManager) LoginStatus(ctx context.Context) error {
	if m == nil || m.Config == nil {
		return fmt.Errorf("missing config")
	}
	if err := m.Config.EnsureCodexCLIHome(); err != nil {
		return err
	}

	builder := &ImageBuilder{Config: m.Config, Logger: m.Logger}
	if err := builder.EnsureImage(ctx); err != nil {
		return err
	}

	cmd := exec.CommandContext(
		ctx,
		"docker", "run", "--rm",
		"--security-opt", "seccomp=unconfined",
		"--name", m.Config.DockerCLIContainerName(),
		"--env", "HOME="+m.Config.ContainerHomePath(),
		"--env", "CODEX_HOME="+m.Config.ContainerHomePath(),
		"--workdir", m.Config.ContainerHomePath(),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", m.Config.CodexCLIHomeRoot(), m.Config.ContainerHomePath()),
		m.Config.DockerImage(),
		"codex", "login", "status",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func (m *CodexManager) RunCLI(ctx context.Context, workdir string, args []string) error {
	if m == nil || m.Config == nil {
		return fmt.Errorf("missing config")
	}
	if err := m.Config.EnsureCodexCLIHome(); err != nil {
		return err
	}

	builder := &ImageBuilder{Config: m.Config, Logger: m.Logger}
	if err := builder.EnsureImage(ctx); err != nil {
		return err
	}

	if strings.TrimSpace(workdir) == "" {
		var err error
		workdir, err = os.Getwd()
		if err != nil {
			return err
		}
	}

	workspaceHostPath, err := m.Config.ResolveWorkspaceHostPath(workdir)
	if err != nil {
		return err
	}

	dockerArgs := []string{
		"run",
		"--rm",
		"-i",
		"--security-opt", "seccomp=unconfined",
		"--name", m.Config.DockerCLIContainerName(),
		"--env", "HOME=" + m.Config.ContainerHomePath(),
		"--env", "CODEX_HOME=" + m.Config.ContainerHomePath(),
		"--workdir", m.Config.ContainerWorkspacePath(),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", workspaceHostPath, m.Config.ContainerWorkspacePath()),
		"--mount", fmt.Sprintf("type=bind,source=%s,target=%s", m.Config.CodexCLIHomeRoot(), m.Config.ContainerHomePath()),
	}
	if isTerminal(os.Stdin) && isTerminal(os.Stdout) {
		dockerArgs = append(dockerArgs, "-t")
	}
	if term := strings.TrimSpace(os.Getenv("TERM")); term != "" {
		dockerArgs = append(dockerArgs, "--env", "TERM="+term)
	}

	dockerArgs = append(dockerArgs, m.Config.DockerImage(), "codex")
	dockerArgs = append(dockerArgs, args...)

	cmd := exec.CommandContext(ctx, "docker", dockerArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	m.logf("running codex cli workspace=%s args=%q", workspaceHostPath, args)
	return cmd.Run()
}

func (m *CodexManager) logf(format string, args ...any) {
	if m.Logger != nil {
		m.Logger.Printf(format, args...)
	}
}

func isTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}
