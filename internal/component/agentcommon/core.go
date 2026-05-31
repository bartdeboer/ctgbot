package agentcommon

import (
	"context"
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
)

const DefaultStopAfterTurnTimeout = 5 * time.Second

// Core holds the shared infrastructure fields common to all agent components.
// Embed this in a component struct to inherit the fields and methods.
type Core struct {
	Registration        coremodel.Component
	Runtime             runtimepkg.ThreadRuntime
	Storage             repository.Storage
	ResolveWorkspace    func(context.Context, coremodel.Chat) (string, error)
	Logger              *log.Logger
	RuntimeImage        string
	RuntimeDockerfile   string
	RuntimeImageUses    *runtimeimage.Target
	RuntimeImageNoCache bool
}

func (c *Core) Logf(format string, args ...any) {
	if c != nil && c.Logger != nil {
		c.Logger.Printf(format, args...)
	}
}

func (c *Core) ProviderThreadID(turnRuntime component.TurnRuntime) (string, error) {
	return ProviderThreadID(c.Registration.ID, turnRuntime)
}

func (c *Core) BindComponentThreadID(turnRuntime component.TurnRuntime, providerThreadID string) error {
	return BindProviderThreadID(c.Registration.ID, turnRuntime, providerThreadID)
}

func (c *Core) StoredProviderThreadID(ctx context.Context, threadID modeluuid.UUID) (string, error) {
	if c == nil || c.Storage == nil {
		return "", fmt.Errorf("missing storage")
	}
	mapping, err := c.Storage.ThreadComponentMappings().GetByThreadAndComponent(ctx, threadID, c.Registration.ID)
	if err != nil || mapping == nil {
		return "", err
	}
	return strings.TrimSpace(mapping.ComponentThreadID), nil
}

func (c *Core) SaveStoredProviderThreadID(ctx context.Context, thread *coremodel.Thread, providerThreadID string) error {
	providerThreadID = strings.TrimSpace(providerThreadID)
	if providerThreadID == "" {
		return nil
	}
	if c == nil || c.Storage == nil {
		return fmt.Errorf("missing storage")
	}
	if thread == nil {
		return fmt.Errorf("missing thread")
	}
	return c.Storage.ThreadComponentMappings().Save(ctx, &coremodel.ThreadComponentMapping{
		ThreadID:          thread.ID,
		ChatID:            thread.ChatID,
		ComponentID:       c.Registration.ID,
		ComponentThreadID: providerThreadID,
	})
}

func (c *Core) RuntimeNotices(ctx context.Context, workspacePath string, threadID modeluuid.UUID) []string {
	return RuntimeNotices(ctx, c.Runtime, workspacePath, threadID, c.Logf)
}

func (c *Core) StopAfterTurn(workspacePath string, threadID modeluuid.UUID, timeout time.Duration) {
	StopAfterTurn(c.Runtime, workspacePath, threadID, timeout, c.Logf)
}

// RefreshThreadRuntime implements component.ThreadRuntimeController.
func (c *Core) RefreshThreadRuntime(ctx context.Context, request component.ThreadRuntimeControlRequest) error {
	if c == nil || c.Runtime == nil {
		return fmt.Errorf("missing runtime")
	}
	return c.Runtime.Refresh(ctx, request.WorkspacePath, request.Thread.ID)
}

// FirstNonEmpty returns the first non-empty string from vals.
func FirstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// WriterOrDiscard returns w if non-nil, otherwise io.Discard.
func WriterOrDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}
