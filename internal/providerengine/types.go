package providerengine

import "github.com/bartdeboer/ctgbot/internal/sandboxengine"

type PrepareSandboxRequest struct {
	ProfilePath         string
	WorkspacePath       string
	ContainerHome       string
	ContainerWorkspace  string
	HostOS              string
	HostbridgeAddr      string
	AllowedHostCommands []string
}

type SandboxSpecRequest struct {
	SandboxName        string
	ProfilePath        string
	WorkspacePath      string
	ContainerHome      string
	ContainerWorkspace string
}

type PromptRequest struct {
	ProviderThreadID   string
	Prompt             string
	ContainerHome      string
	ContainerWorkspace string
}

type PromptResult struct {
	Reply            string
	ProviderThreadID string
}

type Provider interface {
	PrepareSandbox(req PrepareSandboxRequest) error
	SandboxSpec(req SandboxSpecRequest) sandboxengine.Spec
	SendPrompt(req PromptRequest, sbx sandboxengine.Sandbox) (PromptResult, error)
}
