package telegramengine

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/bartdeboer/ctgbot/internal/chatmodel"
)

type debounceKey struct {
	ChatID   int64
	ThreadID int
	UserID   int64
}

type pendingUpdate struct {
	key        debounceKey
	update     chatmodel.TelegramUpdate
	textParts  []string
	timer      *time.Timer
	generation uint64
	firstSeen  time.Time
}

type Debouncer struct {
	Window time.Duration
	Logger *log.Logger
	Next   func(context.Context, chatmodel.TelegramUpdate)

	mu      sync.Mutex
	pending map[debounceKey]*pendingUpdate
}

func NewDebouncer(window time.Duration, logger *log.Logger, next func(context.Context, chatmodel.TelegramUpdate)) *Debouncer {
	return &Debouncer{
		Window: window,
		Logger: logger,
		Next:   next,
	}
}

func (d *Debouncer) Run(ctx context.Context, api TelegramAPI, pollTimeout time.Duration) error {
	if api == nil {
		return fmt.Errorf("missing telegram api")
	}
	defer d.FlushAll()
	return api.Run(ctx, pollTimeout, d.HandleUpdate)
}

func (d *Debouncer) HandleUpdate(ctx context.Context, u chatmodel.TelegramUpdate) {
	if d == nil || d.Next == nil {
		return
	}
	text := strings.TrimSpace(u.Text)
	if text == "" && len(u.Attachments) == 0 {
		return
	}
	if len(u.Attachments) > 0 {
		d.logf("telegram debounce received chat=%d thread=%d user=%d msg=%d text=%q attachments=%d details=%s", u.ChatID, u.ThreadID, u.UserID, u.MessageID, text, len(u.Attachments), formatDebounceAttachments(u.Attachments))
	}
	if d.Window <= 0 {
		d.Next(ctx, u)
		return
	}
	if isTelegramCommand(text) {
		d.flushChatThread(u.ChatID, u.ThreadID)
		d.Next(ctx, u)
		return
	}

	key := debounceKeyForUpdate(u)

	d.mu.Lock()
	if d.pending == nil {
		d.pending = map[debounceKey]*pendingUpdate{}
	}
	pending := d.pending[key]
	if pending == nil {
		pending = &pendingUpdate{
			key:       key,
			update:    u,
			firstSeen: time.Now(),
		}
		d.pending[key] = pending
	} else {
		if len(u.Attachments) > 0 {
			d.logf("telegram debounce merging chat=%d thread=%d user=%d pending_msg=%d new_msg=%d attachments_before=%d attachments_added=%d", u.ChatID, u.ThreadID, u.UserID, pending.update.MessageID, u.MessageID, len(pending.update.Attachments), len(u.Attachments))
		}
		pending.update.ChatTitle = u.ChatTitle
		pending.update.MessageID = u.MessageID
		pending.update.UserID = u.UserID
		pending.update.FirstName = u.FirstName
		pending.update.LastName = u.LastName
		pending.update.Username = u.Username
	}
	if text != "" {
		pending.textParts = append(pending.textParts, text)
	}
	pending.update.Attachments = append(pending.update.Attachments, u.Attachments...)
	pending.generation++
	generation := pending.generation
	if pending.timer != nil {
		pending.timer.Stop()
	}
	pending.timer = time.AfterFunc(d.Window, func() {
		d.flushMatch(key, generation)
	})
	d.mu.Unlock()
}

func (d *Debouncer) FlushAll() {
	for _, pending := range d.takeAllPending() {
		d.emitPending(pending)
	}
}

func (d *Debouncer) flushMatch(key debounceKey, generation uint64) {
	pending := d.takePendingMatch(key, generation)
	if pending == nil {
		return
	}
	d.emitPending(pending)
}

func (d *Debouncer) flushChatThread(chatID int64, threadID int) {
	for _, pending := range d.takePendingChatThread(chatID, threadID) {
		d.emitPending(pending)
	}
}

func (d *Debouncer) emitPending(pending *pendingUpdate) {
	if pending == nil || d == nil || d.Next == nil {
		return
	}
	merged := pending.update
	merged.Text = strings.Join(pending.textParts, "\n\n")
	textCount := len(pending.textParts)
	if strings.TrimSpace(merged.Text) == "" && len(merged.Attachments) == 0 {
		return
	}
	if textCount > 1 {
		d.logf("telegram debounced update merged chat=%d thread=%d user=%d messages=%d final_msg=%d", merged.ChatID, merged.ThreadID, merged.UserID, textCount, merged.MessageID)
	}
	if len(merged.Attachments) > 0 {
		d.logf("telegram debounced attachments chat=%d thread=%d user=%d msg=%d attachments=%d details=%s", merged.ChatID, merged.ThreadID, merged.UserID, merged.MessageID, len(merged.Attachments), formatDebounceAttachments(merged.Attachments))
	}
	d.Next(context.Background(), merged)
}

func (d *Debouncer) takePendingMatch(key debounceKey, generation uint64) *pendingUpdate {
	d.mu.Lock()
	defer d.mu.Unlock()
	pending := d.pending[key]
	if pending == nil || pending.generation != generation {
		return nil
	}
	delete(d.pending, key)
	if pending.timer != nil {
		pending.timer.Stop()
		pending.timer = nil
	}
	return pending
}

func (d *Debouncer) takePendingChatThread(chatID int64, threadID int) []*pendingUpdate {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.pending) == 0 {
		return nil
	}
	keys := make([]debounceKey, 0, len(d.pending))
	for key := range d.pending {
		if sameChatThread(key, chatID, threadID) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	pendingList := make([]*pendingUpdate, 0, len(keys))
	for _, key := range keys {
		pending := d.pending[key]
		delete(d.pending, key)
		if pending == nil {
			continue
		}
		if pending.timer != nil {
			pending.timer.Stop()
			pending.timer = nil
		}
		pendingList = append(pendingList, pending)
	}
	return pendingList
}

func (d *Debouncer) takeAllPending() []*pendingUpdate {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.pending) == 0 {
		return nil
	}
	pendingList := make([]*pendingUpdate, 0, len(d.pending))
	for key, pending := range d.pending {
		delete(d.pending, key)
		if pending == nil {
			continue
		}
		if pending.timer != nil {
			pending.timer.Stop()
			pending.timer = nil
		}
		pendingList = append(pendingList, pending)
	}
	return pendingList
}

func (d *Debouncer) logf(format string, args ...any) {
	if d != nil && d.Logger != nil {
		d.Logger.Printf(format, args...)
	}
}

func debounceKeyForUpdate(u chatmodel.TelegramUpdate) debounceKey {
	return debounceKey{ChatID: u.ChatID, ThreadID: u.ThreadID, UserID: u.UserID}
}

func isTelegramCommand(text string) bool {
	return strings.HasPrefix(strings.TrimSpace(text), "/")
}

func sameChatThread(a debounceKey, chatID int64, threadID int) bool {
	return a.ChatID == chatID && a.ThreadID == threadID
}

func formatDebounceAttachments(attachments []chatmodel.TelegramAttachment) string {
	if len(attachments) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(attachments))
	for _, attachment := range attachments {
		parts = append(parts, fmt.Sprintf("%s:%s(%s)", strings.TrimSpace(attachment.Kind), strings.TrimSpace(attachment.Filename), strings.TrimSpace(attachment.FileID)))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}
