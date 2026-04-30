package codex

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

const (
	DefaultWorkspacePath = "/workspace"
	defaultWorkspaceRoot = ".ctgbot/v2/workspaces"
)

func (c *Component) HandleMessage(ctx context.Context, message coremodel.ThreadMessage) (*coremodel.ThreadMessage, error) {
	prompt := strings.TrimSpace(message.Text)
	if prompt == "" {
		return nil, nil
	}
	if c == nil || c.Config.SandboxManager == nil {
		return nil, fmt.Errorf("missing sandbox manager")
	}

	spec, err := RuntimeSandboxSpec(c.Config, message)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(runtimeWorkspaceHostPath(c.Config, message), 0o755); err != nil {
		return nil, err
	}
	conversation, err := c.loadThreadConversation(ctx, message.ThreadID)
	if err != nil {
		return nil, fmt.Errorf("load codex conversation profile=%s thread=%s: %w", c.Config.ProfileName, message.ThreadID, err)
	}

	sbx := c.Config.SandboxManager.CreateSandbox(spec)
	defer func() { _ = sbx.Stop(context.Background()) }()

	var stdout bytes.Buffer
	var stderr io.Writer = os.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	args := codexExecArgs(prompt, conversation)
	if err := sbx.Exec(ctx, &stdout, stderr, "codex", args...); err != nil {
		return nil, fmt.Errorf("codex exec profile=%s thread=%s: %w", c.Config.ProfileName, message.ThreadID, err)
	}
	result, err := parseCodexJSONOutput(stdout.String())
	if err != nil {
		return nil, fmt.Errorf("parse codex json profile=%s thread=%s: %w", c.Config.ProfileName, message.ThreadID, err)
	}
	conversation = mergeThreadConversation(conversation, result.Conversation)
	if err := c.saveThreadConversation(ctx, message.ThreadID, conversation); err != nil {
		return nil, fmt.Errorf("save codex conversation profile=%s thread=%s: %w", c.Config.ProfileName, message.ThreadID, err)
	}
	if result.Reply == "" {
		return nil, nil
	}
	return &coremodel.ThreadMessage{
		Kind:       coremodel.MessageKindAgent,
		SourceType: ComponentType,
		ActorID:    ComponentType,
		ActorLabel: "Codex",
		Text:       result.Reply,
	}, nil
}

func codexExecArgs(prompt string, conversation CodexThreadConversation) []string {
	args := []string{
		"-a", "never",
		"-s", "workspace-write",
		"exec",
		"--json",
		"--skip-git-repo-check",
		"--add-dir", DefaultWorkspacePath,
		"-C", DefaultWorkspacePath,
	}
	if resumeID := conversation.ResumeID(); resumeID != "" {
		return append(args, "resume", resumeID, prompt)
	}
	return append(args, prompt)
}

func mergeThreadConversation(current CodexThreadConversation, next CodexThreadConversation) CodexThreadConversation {
	if threadID := strings.TrimSpace(next.ThreadID); threadID != "" {
		current.ThreadID = threadID
	}
	if sessionID := strings.TrimSpace(next.SessionID); sessionID != "" {
		current.SessionID = sessionID
	}
	return current
}

func RuntimeSandboxSpec(config Config, message coremodel.ThreadMessage) (*sandboxengine.SandboxSpec, error) {
	profileHostPath := strings.TrimSpace(config.ProfileHostPath)
	if profileHostPath == "" {
		return nil, fmt.Errorf("missing profile host path")
	}
	profileContainerPath := strings.TrimSpace(config.ProfileContainerPath)
	if profileContainerPath == "" {
		profileContainerPath = DefaultProfilePath
	}
	workspaceHostPath := runtimeWorkspaceHostPath(config, message)

	return sandboxengine.NewBuilder(runtimeSandboxName(message)).
		Image(componentImage(config)).
		Workdir(DefaultWorkspacePath).
		Env([]string{
			"HOME=" + profileContainerPath,
			"CODEX_HOME=" + profileContainerPath,
		}).
		Mounts([]sandboxengine.Mount{
			{Source: profileHostPath, Target: profileContainerPath},
			{Source: workspaceHostPath, Target: DefaultWorkspacePath},
		}).
		SecurityOpts([]string{"seccomp=unconfined"}).
		Cmd([]string{"tail", "-f", "/dev/null"}).
		Build(), nil
}

func runtimeSandboxName(message coremodel.ThreadMessage) string {
	name := "ctgbot-v2-codex"
	if !message.ThreadID.IsNull() {
		name += "-" + safeName(message.ThreadID.String(), "thread")
	}
	return name
}

func runtimeWorkspaceHostPath(config Config, message coremodel.ThreadMessage) string {
	root := strings.TrimSpace(config.WorkspaceRoot)
	if root == "" {
		root = defaultWorkspaceRoot
	}
	threadID := "default"
	if !message.ThreadID.IsNull() {
		threadID = message.ThreadID.String()
	}
	return filepath.Join(root, threadID)
}

func componentImage(config Config) string {
	if image := strings.TrimSpace(config.Image); image != "" {
		return image
	}
	return DefaultImage
}
