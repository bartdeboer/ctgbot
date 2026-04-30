package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/v2/coremodel"
)

type CodexThreadConversation struct {
	ThreadID  string `json:"thread_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
}

func (c CodexThreadConversation) ResumeID() string {
	if sessionID := strings.TrimSpace(c.SessionID); sessionID != "" {
		return sessionID
	}
	return strings.TrimSpace(c.ThreadID)
}

func (c CodexThreadConversation) IsZero() bool {
	return strings.TrimSpace(c.ThreadID) == "" && strings.TrimSpace(c.SessionID) == ""
}

func decodeThreadConversation(stateJSON string) (CodexThreadConversation, error) {
	stateJSON = strings.TrimSpace(stateJSON)
	if stateJSON == "" {
		return CodexThreadConversation{}, nil
	}
	var conversation CodexThreadConversation
	if err := json.Unmarshal([]byte(stateJSON), &conversation); err != nil {
		return CodexThreadConversation{}, err
	}
	return conversation, nil
}

func encodeThreadConversation(conversation CodexThreadConversation) (string, error) {
	body, err := json.Marshal(conversation)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (c *Component) loadThreadConversation(ctx context.Context, threadID modeluuid.UUID) (CodexThreadConversation, error) {
	if c == nil || c.Config.StateStore == nil || threadID.IsNull() || c.ProfileName() == "" {
		return CodexThreadConversation{}, nil
	}
	state, err := c.Config.StateStore.Get(ctx, threadID, ComponentType, c.ProfileName())
	if err != nil || state == nil {
		return CodexThreadConversation{}, err
	}
	return decodeThreadConversation(state.StateJSON)
}

func (c *Component) saveThreadConversation(ctx context.Context, threadID modeluuid.UUID, conversation CodexThreadConversation) error {
	if c == nil || c.Config.StateStore == nil || threadID.IsNull() || c.ProfileName() == "" || conversation.IsZero() {
		return nil
	}
	stateJSON, err := encodeThreadConversation(conversation)
	if err != nil {
		return err
	}
	return c.Config.StateStore.Save(ctx, &coremodel.ThreadComponentState{
		ThreadID:      threadID,
		ComponentType: ComponentType,
		ProfileName:   c.ProfileName(),
		StateJSON:     stateJSON,
	})
}

type codexJSONResult struct {
	Reply        string
	Conversation CodexThreadConversation
}

type codexJSONEvent struct {
	Type      string `json:"type"`
	ThreadID  string `json:"thread_id"`
	SessionID string `json:"session_id"`
	Item      struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"item"`
}

func parseCodexJSONOutput(output string) (codexJSONResult, error) {
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	var result codexJSONResult
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var event codexJSONEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			return codexJSONResult{}, err
		}
		if threadID := strings.TrimSpace(event.ThreadID); threadID != "" {
			result.Conversation.ThreadID = threadID
		}
		if sessionID := strings.TrimSpace(event.SessionID); sessionID != "" {
			result.Conversation.SessionID = sessionID
		}
		if event.Type == "item.completed" && event.Item.Type == "agent_message" {
			result.Reply = strings.TrimSpace(event.Item.Text)
		}
	}
	if err := scanner.Err(); err != nil {
		return codexJSONResult{}, err
	}
	return result, nil
}
