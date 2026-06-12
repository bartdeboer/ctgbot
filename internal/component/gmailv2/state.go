package gmailv2

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type mailboxState struct {
	MailboxEmail  string    `json:"mailbox_email,omitempty"`
	HistoryID     uint64    `json:"history_id,omitempty"`
	LastMessageID string    `json:"last_message_id,omitempty"`
	LastSyncedAt  time.Time `json:"last_synced_at,omitempty"`
}

func (c *Component) loadState() (mailboxState, error) {
	path := c.statePath()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return mailboxState{}, nil
		}
		return mailboxState{}, fmt.Errorf("read gmailv2 state %s: %w", path, err)
	}
	var state mailboxState
	if err := json.Unmarshal(data, &state); err != nil {
		return mailboxState{}, fmt.Errorf("read gmailv2 state %s: %w", path, err)
	}
	state.MailboxEmail = strings.TrimSpace(state.MailboxEmail)
	state.LastMessageID = strings.TrimSpace(state.LastMessageID)
	return state, nil
}

func (c *Component) saveState(state mailboxState) error {
	path := c.statePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	state.MailboxEmail = strings.TrimSpace(state.MailboxEmail)
	state.LastMessageID = strings.TrimSpace(state.LastMessageID)
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("encode gmailv2 state: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("write gmailv2 state: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("commit gmailv2 state: %w", err)
	}
	return nil
}

func (c *Component) statePath() string {
	if c == nil {
		return StateFilename
	}
	return filepath.Join(strings.TrimSpace(c.profile.Path), StateFilename)
}
