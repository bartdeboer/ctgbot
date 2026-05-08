package telegram

import (
	"strings"
	"time"
)

type TelegramAttachment struct {
	Kind     string
	FileID   string
	Filename string
}

type TelegramUpdate struct {
	ChatID      int64
	ChatTitle   string
	MessageID   int
	ThreadID    int
	Text        string
	Attachments []TelegramAttachment
	FirstName   string
	LastName    string
	Username    string
	UserID      int64

	ResponseText string
	ErrorText    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (t TelegramUpdate) UserLabel() string {
	if t.Username != "" {
		return "@" + t.Username
	}
	return strings.TrimSpace(t.FirstName + " " + t.LastName)
}
