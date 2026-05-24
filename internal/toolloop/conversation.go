package toolloop

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type Conversation struct {
	ID        string             `json:"id"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	Model     string             `json:"model,omitempty"`
	Messages  []Message          `json:"messages,omitempty"`
	Turns     []ConversationTurn `json:"turns,omitempty"`
}

type ConversationTurn struct {
	Prompt     string      `json:"prompt"`
	Status     string      `json:"status,omitempty"`
	Text       string      `json:"text,omitempty"`
	Error      string      `json:"error,omitempty"`
	Iterations int         `json:"iterations,omitempty"`
	Trace      []TraceStep `json:"trace,omitempty"`
	StartedAt  time.Time   `json:"started_at"`
	EndedAt    time.Time   `json:"ended_at"`
}

type ConversationStore struct {
	Dir string
}

func DefaultConversationDir() string {
	if dir := strings.TrimSpace(getenv("TOOLLOOP_CONVERSATION_DIR")); dir != "" {
		return dir
	}
	if home := strings.TrimSpace(getenv("HOME")); home != "" {
		return filepath.Join(home, ".toolloop", "conversations")
	}
	return filepath.Join(".", ".toolloop", "conversations")
}

func NewConversationStore(dir string) ConversationStore {
	if strings.TrimSpace(dir) == "" {
		dir = DefaultConversationDir()
	}
	return ConversationStore{Dir: dir}
}

func (s ConversationStore) New() Conversation {
	now := time.Now().UTC()
	return Conversation{ID: modeluuid.New().String(), CreatedAt: now, UpdatedAt: now}
}

func (s ConversationStore) Load(id string) (Conversation, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return Conversation{}, errors.New("missing conversation id")
	}
	data, err := os.ReadFile(s.path(id))
	if err != nil {
		return Conversation{}, err
	}
	var conversation Conversation
	if err := json.Unmarshal(data, &conversation); err != nil {
		return Conversation{}, err
	}
	if strings.TrimSpace(conversation.ID) == "" {
		conversation.ID = id
	}
	return conversation, nil
}

func (s ConversationStore) Save(conversation Conversation) error {
	conversation.ID = strings.TrimSpace(conversation.ID)
	if conversation.ID == "" {
		return errors.New("missing conversation id")
	}
	if conversation.CreatedAt.IsZero() {
		conversation.CreatedAt = time.Now().UTC()
	}
	conversation.UpdatedAt = time.Now().UTC()
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(conversation, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(s.path(conversation.ID), data, 0o600)
}

func (s ConversationStore) path(id string) string {
	return filepath.Join(s.Dir, strings.TrimSpace(id)+".json")
}
