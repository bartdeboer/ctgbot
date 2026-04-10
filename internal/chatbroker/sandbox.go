package chatbroker

import (
	"context"
	"runtime"

	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

func (b *Broker) newSandbox(conv *Thread) *sandboxengine.Sandbox {
	sbx := b.sandboxManager().NewSandbox(conv.ContainerName)

	chatID, threadID, _ := b.Config.ParseChatContainerName(conv.ContainerName)

	sbx.WorkspaceDir = conv.WorkspaceHost
	sbx.ProfileDir = conv.HomeHost
	sbx.ContainerWorkspace = conv.ContainerWorkspace
	sbx.ContainerHome = conv.ContainerHome
	sbx.DeveloperInstructions = b.developerInstructions(chatID, conv)
	sbx.Hostname = conv.ContainerName
	sbx.Image = b.Config.DockerImage()
	sbx.Workdir = conv.ContainerWorkspace
	sbx.Labels = map[string]string{
		"ctgbot.managed":   "true",
		"ctgbot.chat_id":   chatID.String(),
		"ctgbot.thread_id": threadID.String(),
	}
	sbx.Env = []string{
		"HOME=" + conv.ContainerHome,
		"CODEX_HOME=" + conv.ContainerHome,
		"GOCACHE=/tmp/go-build-cache",
		"GOMODCACHE=/tmp/go-mod-cache",
		"GOPATH=/tmp/go",
		"XDG_CACHE_HOME=/tmp/.cache",
		"HOSTBRIDGE_ADDR=" + b.Config.ContainerHostbridgeTCPAddr(),
		"HOSTBRIDGE_TLS_DIR=" + b.Config.ContainerHostbridgeTLSDir(),
	}
	if b.Config != nil {
		identity := b.Config.HostGitIdentity(context.Background())
		if identity.Name != "" {
			sbx.Env = append(sbx.Env,
				"GIT_AUTHOR_NAME="+identity.Name,
				"GIT_COMMITTER_NAME="+identity.Name,
			)
		}
		if identity.Email != "" {
			sbx.Env = append(sbx.Env,
				"GIT_AUTHOR_EMAIL="+identity.Email,
				"GIT_COMMITTER_EMAIL="+identity.Email,
			)
		}
	}
	sbx.Mounts = []sandboxengine.Mount{
		{Source: conv.WorkspaceHost, Target: conv.ContainerWorkspace},
		{Source: conv.HomeHost, Target: conv.ContainerHome},
		{
			Source:   b.Config.ChatTLSDirByID(chatID),
			Target:   b.Config.ContainerHostbridgeTLSDir(),
			ReadOnly: true,
		},
	}
	sbx.SecurityOpts = []string{"seccomp=unconfined"}
	sbx.Cmd = []string{"tail", "-f", "/dev/null"}

	if runtime.GOOS == "linux" {
		sbx.AddHosts = []string{"host.docker.internal:host-gateway"}
	}
	return sbx
}
