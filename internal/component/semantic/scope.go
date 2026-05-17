package semantic

import (
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type scopeFlags struct {
	Chat   string
	Thread string
	All    bool
}

type scope struct {
	ChatID   modeluuid.UUID
	ThreadID modeluuid.UUID
	All      bool
}

func resolveScope(ctx commandengine.Context, flags scopeFlags) (scope, error) {
	if flags.All {
		if strings.TrimSpace(flags.Chat) != "" || strings.TrimSpace(flags.Thread) != "" {
			return scope{}, fmt.Errorf("--all cannot be combined with --chat or --thread")
		}
		return scope{All: true}, nil
	}
	if strings.TrimSpace(flags.Chat) != "" && strings.TrimSpace(flags.Thread) != "" {
		return scope{}, fmt.Errorf("--chat and --thread are mutually exclusive")
	}
	if strings.TrimSpace(flags.Chat) != "" {
		id, err := parseRequiredUUID("--chat", flags.Chat)
		if err != nil {
			return scope{}, err
		}
		return scope{ChatID: id}, nil
	}
	if strings.TrimSpace(flags.Thread) != "" {
		id, err := parseRequiredUUID("--thread", flags.Thread)
		if err != nil {
			return scope{}, err
		}
		return scope{ThreadID: id}, nil
	}
	threadID := ctx.ThreadID
	if threadID.IsNull() {
		threadID = ctx.SandboxID
	}
	if threadID.IsNull() {
		return scope{}, fmt.Errorf("missing thread id")
	}
	return scope{ThreadID: threadID}, nil
}

func resolveIndexScope(ctx commandengine.Context, flags scopeFlags) (scope, error) {
	return resolveScope(ctx, flags)
}

func parseRequiredUUID(name string, value string) (modeluuid.UUID, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return modeluuid.UUID{}, fmt.Errorf("missing %s", name)
	}
	id, err := modeluuid.Parse(value)
	if err != nil {
		return modeluuid.UUID{}, fmt.Errorf("parse %s: %w", name, err)
	}
	if id.IsNull() {
		return modeluuid.UUID{}, fmt.Errorf("missing %s", name)
	}
	return id, nil
}

func scopeText(scope scope) string {
	switch {
	case scope.All:
		return "all"
	case !scope.ThreadID.IsNull():
		return "thread " + scope.ThreadID.String()
	case !scope.ChatID.IsNull():
		return "chat " + scope.ChatID.String()
	default:
		return "current thread"
	}
}
