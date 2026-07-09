package outlook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type mailboxState struct {
	MailboxEmail string    `json:"mailbox_email,omitempty"`
	SeenIDs      []string  `json:"seen_ids,omitempty"`
	LastSyncedAt time.Time `json:"last_synced_at,omitempty"`
}

func (c *Component) loadState() (mailboxState, error) {
	path := filepath.Join(strings.TrimSpace(c.profile.Path), StateFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return mailboxState{}, nil
		}
		return mailboxState{}, fmt.Errorf("read outlook state %s: %w", path, err)
	}
	var state mailboxState
	if err := json.Unmarshal(data, &state); err != nil {
		return mailboxState{}, fmt.Errorf("parse outlook state %s: %w", path, err)
	}
	state.MailboxEmail = strings.TrimSpace(state.MailboxEmail)
	return state, nil
}

func (c *Component) saveState(state mailboxState) error {
	path := filepath.Join(strings.TrimSpace(c.profile.Path), StateFilename)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}
