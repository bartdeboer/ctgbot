package chatbroker

import (
	"context"
	"runtime"

	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

func (b *Broker) newSandboxSpec(conv *Thread) *sandboxengine.SandboxSpec {
	containerName := conv.ContainerName(b.Config)
	spec := sandboxengine.NewBuilder(containerName).
		InteractiveInterruptEnabled(b.Config.Chat(conv.ChatID).InteractiveInterruptEnabled()).
		WorkspaceDir(conv.WorkspaceHost).
		ProfileDir(conv.HomeHost).
		ContainerWorkspace(conv.ContainerWorkspace).
		ContainerHome(conv.ContainerHome).
		DeveloperInstructions(b.developerInstructions(conv.ChatID, conv)).
		Hostname(containerName).
		Image(b.Config.Docker().Image()).
		Workdir(conv.ContainerWorkspace).
		UserMode(b.Config.Chat(conv.ChatID).ContainerUserMode()).
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
				Source:   b.Config.Chat(conv.ChatID).Profile().TLSDir(),
				Target:   b.Config.Docker().ContainerHostbridgeTLSDir(),
				ReadOnly: true,
			},
		}).
		SecurityOpts([]string{"seccomp=unconfined"}).
		Cmd([]string{"tail", "-f", "/dev/null"}).
		AddHosts(b.sandboxAddHosts())

	if gpus := b.Config.Chat(conv.ChatID).GPUs(); gpus != "" {
		spec = spec.GPUs(gpus)
	}

	return spec.Build()
}

func (b *Broker) sandboxForThread(conv *Thread) *sandboxengine.Sandbox {
	if conv == nil {
		return nil
	}
	return b.sandboxManager().CreateSandbox(b.newSandboxSpec(conv))
}

func (b *Broker) sandboxEnv(conv *Thread) []string {
	env := []string{
		"HOME=" + conv.ContainerHome,
		"CODEX_HOME=" + conv.ContainerHome,
		"GOCACHE=/tmp/go-build-cache",
		"GOMODCACHE=/tmp/go-mod-cache",
		"GOPATH=/tmp/go",
		"XDG_CACHE_HOME=/tmp/.cache",
		"HOSTBRIDGE_ADDR=" + b.Config.Docker().ContainerHostbridgeTCPAddr(),
		"HOSTBRIDGE_TLS_DIR=" + b.Config.Docker().ContainerHostbridgeTLSDir(),
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
