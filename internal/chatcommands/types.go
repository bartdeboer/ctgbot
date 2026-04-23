package chatcommands

import (
	"context"

	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type Command interface {
	isCommand()
}

type Request struct {
	ThreadID  modeluuid.UUID
	SandboxID modeluuid.UUID
	Context   CommandContext
	Command   Command
}

type CommandContext struct {
	IsRoot bool
}

type Result struct {
	Text    string
	Session *SessionInfo
}

type SessionInfo struct {
	ThreadID  modeluuid.UUID
	Container string
	Workspace string
}

type Provider interface {
	SendPayload(ctx context.Context, sandboxID modeluuid.UUID, payload messenger.OutboundPayload) error
	StartSession(ctx context.Context, chatID modeluuid.UUID, workspace string, replace bool) (SessionInfo, error)
	StopActiveSession(ctx context.Context, threadID modeluuid.UUID) error
	Status(ctx context.Context, threadID modeluuid.UUID) (string, error)
	RefreshActiveSession(ctx context.Context, threadID modeluuid.UUID) error
	PurgeActiveSession(ctx context.Context, threadID modeluuid.UUID) error
	ResolveThreadIDBySandboxID(ctx context.Context, sandboxID modeluuid.UUID) (*modeluuid.UUID, error)
	List(ctx context.Context, threadID modeluuid.UUID, cmdctx CommandContext) (string, error)
	Set(ctx context.Context, threadID modeluuid.UUID, cmdctx CommandContext, key, value string) (string, error)
	RefreshContainer(ctx context.Context, threadID modeluuid.UUID) (string, error)
	PurgeChat(ctx context.Context, threadID modeluuid.UUID) (string, error)
	InterruptTurn(ctx context.Context, threadID modeluuid.UUID) (string, error)
	Upgrade(ctx context.Context, threadID modeluuid.UUID) (string, error)
	Quit(ctx context.Context, threadID modeluuid.UUID) (string, error)
}

type Runner interface {
	Execute(ctx context.Context, req Request) (Result, error)
}

type HostCommandRunner interface {
	ExecuteRunCommand(ctx context.Context, req Request, cmd RunCommand) (Result, error)
}

type RunCommand struct {
	Command string
	Args    []string
	Stdin   []byte
	Cwd     string
	Env     map[string]string
	Timeout int
}

func (RunCommand) isCommand() {}

type SendMedia struct {
	Filename    string
	Caption     string
	ContentType string
	Syntax      string
	Content     []byte
}

func (SendMedia) isCommand() {}

type ConfigList struct{}

func (ConfigList) isCommand() {}

type ConfigSet struct {
	Setting string
	Value   string
}

func (ConfigSet) isCommand() {}

type StartSession struct {
	ChatID    modeluuid.UUID
	Workspace string
	Replace   bool
}

func (StartSession) isCommand() {}

type StopActiveSession struct{}

func (StopActiveSession) isCommand() {}

type RefreshActiveSession struct{}

func (RefreshActiveSession) isCommand() {}

type PurgeActiveSession struct{}

func (PurgeActiveSession) isCommand() {}

type RefreshContainer struct{}

func (RefreshContainer) isCommand() {}

type PurgeChat struct{}

func (PurgeChat) isCommand() {}

type InterruptTurn struct{}

func (InterruptTurn) isCommand() {}

type Upgrade struct{}

func (Upgrade) isCommand() {}

type Quit struct{}

func (Quit) isCommand() {}

type Stop struct{}

func (Stop) isCommand() {}

type Status struct{}

func (Status) isCommand() {}

type Help struct{}

func (Help) isCommand() {}

type DeprecatedNew struct{}

func (DeprecatedNew) isCommand() {}
