package chatbroker

import (
	"context"
	"runtime"

	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

func (b *Broker) newSandbox(conv *Thread) *sandboxengine.Sandbox {
	containerName := conv.ContainerName(b.Config)
	builder := sandboxengine.NewBuilder(b.sandboxManager(), containerName).
		WorkspaceDir(conv.WorkspaceHost).
		ProfileDir(conv.HomeHost).
		ContainerWorkspace(conv.ContainerWorkspace).
		ContainerHome(conv.ContainerHome).
		DeveloperInstructions(b.developerInstructions(conv.ChatID, conv)).
		Hostname(containerName).
		Image(b.Config.DockerImage()).
		Workdir(conv.ContainerWorkspace).
		Labels(map[string]string{
			"ctgbot.managed":   "true",
			"ctgbot.chat_id":   conv.ChatID.String(),
			"ctgbot.thread_id": conv.ID.String(),
		}).
		Env(b.sandboxEnv(conv)).
		Mounts([]sandboxengine.Mount{
			{Source: conv.WorkspaceHost, Target: conv.ContainerWorkspace},
			{Source: conv.HomeHost, Target: conv.ContainerHome},
			{
				Source:   b.Config.ChatTLSDirByID(conv.ChatID),
				Target:   b.Config.ContainerHostbridgeTLSDir(),
				ReadOnly: true,
			},
		}).
		SecurityOpts([]string{"seccomp=unconfined"}).
		Cmd([]string{"tail", "-f", "/dev/null"}).
		AddHosts(b.sandboxAddHosts())

	if gpus := b.Config.ChatGPUsByID(conv.ChatID); gpus != "" {
		builder = builder.GPUs(gpus)
	}

	return builder.Build()
}

func (b *Broker) sandboxEnv(conv *Thread) []string {
	env := []string{
		"HOME=" + conv.ContainerHome,
		"CODEX_HOME=" + conv.ContainerHome,
		"GOCACHE=/tmp/go-build-cache",
		"GOMODCACHE=/tmp/go-mod-cache",
		"GOPATH=/tmp/go",
		"XDG_CACHE_HOME=/tmp/.cache",
		"HOSTBRIDGE_ADDR=" + b.Config.ContainerHostbridgeTCPAddr(),
		"HOSTBRIDGE_TLS_DIR=" + b.Config.ContainerHostbridgeTLSDir(),
		"CTGBOT_SANDBOX_ID=" + conv.ID.String(),
	}
	if b.Config == nil {
		return env
	}

	identity := b.Config.HostGitIdentity(context.Background())
	if identity.Name != "" {
		env = append(env,
			"GIT_AUTHOR_NAME="+identity.Name,
			"GIT_COMMITTER_NAME="+identity.Name,
		)
	}
	if identity.Email != "" {
		env = append(env,
			"GIT_AUTHOR_EMAIL="+identity.Email,
			"GIT_COMMITTER_EMAIL="+identity.Email,
		)
	}
	return env
}

func (b *Broker) sandboxAddHosts() []string {
	if runtime.GOOS != "linux" {
		return nil
	}
	return []string{"host.docker.internal:host-gateway"}
}
