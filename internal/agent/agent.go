package agent

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

type TurnResult struct {
	Reply            string
	ProviderThreadID string
}

type TurnOptions struct {
	Model           string
	ReasoningEffort string
}

type Agent interface {
	Name() string
	SetupEnvironment(ctx context.Context, sbx *sandboxengine.Sandbox) error
	HandleTurn(ctx context.Context, sbx *sandboxengine.Sandbox, output OutputHandler, providerThreadID string, prompt string) (TurnResult, error)
}

type OptionAgent interface {
	HandleTurnWithOptions(ctx context.Context, sbx *sandboxengine.Sandbox, output OutputHandler, providerThreadID string, prompt string, options TurnOptions) (TurnResult, error)
}

type OutputHandler interface {
	Send(ctx context.Context, payload messenger.OutboundPayload) error
}

type PurgingAgent interface {
	Purge(ctx context.Context, sbx *sandboxengine.Sandbox, providerThreadID string) error
}

type SkillInstallingAgent interface {
	InstallSkill(ctx context.Context, sbx *sandboxengine.Sandbox, skillDir string) error
}
