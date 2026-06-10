package timedintent

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

const (
	KindHeartbeat   = "heartbeat"
	KindWake        = "wake"
	KindCron        = "cron"
	KindMaintenance = "maintenance"

	KeyDefault = "default"

	DeliveryTurn        = "turn"
	DeliveryMaintenance = "maintenance"

	DefaultDueLimit = 100
)

type WakeDeliverer interface {
	ThreadBusy(threadID modeluuid.UUID) bool
	DeliverWake(ctx context.Context, threadID modeluuid.UUID, text string) error
}

type UpdateFeedProvider interface {
	UpdateFeeds(ctx context.Context) ([]component.UpdateFeed, error)
}

type Service struct {
	Intents        repository.TimedIntentRepository
	Deliverer      WakeDeliverer
	UpdateProvider UpdateFeedProvider
	Now            func() time.Time
	Logf           func(format string, args ...any)
}

type RunDueResult struct {
	Due         int
	Delivered   int
	SkippedBusy int
	Failed      int
	Expired     int
}

func New(repo repository.TimedIntentRepository, deliverer WakeDeliverer, updates UpdateFeedProvider, logf func(format string, args ...any)) *Service {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	return &Service{Intents: repo, Deliverer: deliverer, UpdateProvider: updates, Logf: logf}
}

func (s *Service) now() time.Time {
	if s != nil && s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

func (s *Service) StartHeartbeat(ctx context.Context, threadID modeluuid.UUID, every string, owner coremodel.Actor) (*coremodel.TimedIntent, error) {
	if s == nil || s.Intents == nil {
		return nil, fmt.Errorf("missing timed intent repository")
	}
	if threadID.IsNull() {
		return nil, fmt.Errorf("missing thread id")
	}
	every = strings.TrimSpace(every)
	duration, err := time.ParseDuration(every)
	if err != nil {
		return nil, fmt.Errorf("parse heartbeat interval: %w", err)
	}
	if duration <= 0 {
		return nil, fmt.Errorf("heartbeat interval must be positive")
	}
	now := s.now()
	next := now.Add(duration)
	intent := coremodel.TimedIntent{
		TargetThreadID: threadID,
		OwnerThreadID:  threadID,
		OwnerActorID:   strings.TrimSpace(owner.ID),
		Kind:           KindHeartbeat,
		Key:            KeyDefault,
		Enabled:        true,
		NextDueAt:      &next,
		Every:          every,
		Delivery:       DeliveryTurn,
		Label:          "heartbeat",
		LastStatus:     coremodel.TimedIntentStatusNever,
	}
	if err := s.Intents.UpsertByTargetKindKey(ctx, &intent); err != nil {
		return nil, err
	}
	return &intent, nil
}

func (s *Service) StopHeartbeat(ctx context.Context, threadID modeluuid.UUID) (bool, error) {
	if s == nil || s.Intents == nil {
		return false, fmt.Errorf("missing timed intent repository")
	}
	if threadID.IsNull() {
		return false, fmt.Errorf("missing thread id")
	}
	return s.Intents.DeleteByTargetKindKey(ctx, threadID, KindHeartbeat, KeyDefault)
}

func (s *Service) Heartbeat(ctx context.Context, threadID modeluuid.UUID) (*coremodel.TimedIntent, bool, error) {
	if s == nil || s.Intents == nil {
		return nil, false, fmt.Errorf("missing timed intent repository")
	}
	intent, err := s.Intents.GetByTargetKindKey(ctx, threadID, KindHeartbeat, KeyDefault)
	if err != nil {
		var notFound *repository.ShortIDNotFoundError
		if errors.As(err, &notFound) {
			return nil, false, nil
		}
		return nil, false, err
	}
	return intent, true, nil
}

func (s *Service) ResetHeartbeatFloor(ctx context.Context, threadID modeluuid.UUID, now time.Time) error {
	if s == nil || s.Intents == nil || threadID.IsNull() {
		return nil
	}
	intent, found, err := s.Heartbeat(ctx, threadID)
	if err != nil || !found || intent == nil || !intent.Enabled {
		return err
	}
	every, err := time.ParseDuration(strings.TrimSpace(intent.Every))
	if err != nil || every <= 0 {
		return err
	}
	if now.IsZero() {
		now = s.now()
	}
	next := now.UTC().Add(every)
	intent.NextDueAt = &next
	return s.Intents.Save(ctx, intent)
}

func (s *Service) RunDue(ctx context.Context) (RunDueResult, error) {
	if s == nil || s.Intents == nil {
		return RunDueResult{}, fmt.Errorf("missing timed intent repository")
	}
	now := s.now()
	due, err := s.Intents.ListDue(ctx, now, DefaultDueLimit)
	if err != nil {
		return RunDueResult{}, err
	}
	result := RunDueResult{Due: len(due)}
	if len(due) == 0 {
		return result, nil
	}

	var turnIntents []coremodel.TimedIntent
	for _, intent := range due {
		if expiredBeforeDelivery(intent, now) {
			if err := s.expireIntent(ctx, intent, now); err != nil {
				result.Failed++
				s.Logf("timed intent expire failed kind=%s key=%s err=%v", intent.Kind, intent.Key, err)
			} else {
				result.Expired++
			}
			continue
		}
		switch strings.TrimSpace(intent.Delivery) {
		case DeliveryMaintenance:
			result.Delivered++
			if err := s.finishIntent(ctx, intent, nil, now); err != nil {
				result.Failed++
				s.Logf("timed maintenance intent finish failed kind=%s key=%s err=%v", intent.Kind, intent.Key, err)
			}
		default:
			turnIntents = append(turnIntents, intent)
		}
	}

	for threadID, group := range groupByTarget(turnIntents) {
		if s.Deliverer == nil {
			result.Failed += len(group)
			for _, intent := range group {
				_ = s.finishIntent(ctx, intent, fmt.Errorf("missing wake deliverer"), now)
			}
			continue
		}
		if s.Deliverer.ThreadBusy(threadID) {
			result.SkippedBusy += len(group)
			continue
		}
		text := s.composeWakeText(ctx, threadID, group)
		if err := s.Deliverer.DeliverWake(ctx, threadID, text); err != nil {
			result.Failed += len(group)
			for _, intent := range group {
				_ = s.finishIntent(ctx, intent, err, now)
			}
			continue
		}
		result.Delivered += len(group)
		for _, intent := range group {
			if err := s.finishIntent(ctx, intent, nil, now); err != nil {
				result.Failed++
				s.Logf("timed intent finish failed kind=%s key=%s err=%v", intent.Kind, intent.Key, err)
			}
		}
	}
	return result, nil
}

func (s *Service) finishIntent(ctx context.Context, intent coremodel.TimedIntent, runErr error, now time.Time) error {
	intent.LastRunAt = &now
	if runErr != nil {
		intent.LastStatus = coremodel.TimedIntentStatusFailed
		intent.LastError = runErr.Error()
		return s.Intents.Save(ctx, &intent)
	}
	intent.RunCount++
	intent.LastStatus = coremodel.TimedIntentStatusSuccess
	intent.LastError = ""

	if shouldExpire(intent, now) {
		intent.Enabled = false
		intent.NextDueAt = nil
		intent.LastStatus = coremodel.TimedIntentStatusExpired
		return s.Intents.Save(ctx, &intent)
	}
	next, done, err := nextAfterDelivery(intent, now)
	if err != nil {
		intent.LastStatus = coremodel.TimedIntentStatusFailed
		intent.LastError = err.Error()
		return s.Intents.Save(ctx, &intent)
	}
	if done {
		intent.Enabled = false
		intent.NextDueAt = nil
		return s.Intents.Save(ctx, &intent)
	}
	intent.NextDueAt = next
	return s.Intents.Save(ctx, &intent)
}

func (s *Service) expireIntent(ctx context.Context, intent coremodel.TimedIntent, now time.Time) error {
	intent.Enabled = false
	intent.NextDueAt = nil
	intent.LastRunAt = &now
	intent.LastStatus = coremodel.TimedIntentStatusExpired
	intent.LastError = ""
	return s.Intents.Save(ctx, &intent)
}

func expiredBeforeDelivery(intent coremodel.TimedIntent, now time.Time) bool {
	if intent.MaxRuns > 0 && intent.RunCount >= intent.MaxRuns {
		return true
	}
	return intent.ExpiresAt != nil && !now.Before(intent.ExpiresAt.UTC())
}

func shouldExpire(intent coremodel.TimedIntent, now time.Time) bool {
	if intent.MaxRuns > 0 && intent.RunCount >= intent.MaxRuns {
		return true
	}
	return intent.ExpiresAt != nil && !now.Before(intent.ExpiresAt.UTC())
}

func nextAfterDelivery(intent coremodel.TimedIntent, now time.Time) (*time.Time, bool, error) {
	every := strings.TrimSpace(intent.Every)
	if every == "" {
		return nil, true, nil
	}
	duration, err := time.ParseDuration(every)
	if err != nil {
		return nil, false, fmt.Errorf("parse recurrence interval: %w", err)
	}
	if duration <= 0 {
		return nil, false, fmt.Errorf("recurrence interval must be positive")
	}
	next := now.UTC().Add(duration)
	return &next, false, nil
}

func (s *Service) composeWakeText(ctx context.Context, threadID modeluuid.UUID, intents []coremodel.TimedIntent) string {
	var lines []string
	lines = append(lines, "Reasons:")
	for _, intent := range intents {
		reason := intentReason(intent)
		if intent.Kind == KindHeartbeat {
			updates := s.collectUpdates(ctx, threadID)
			if len(updates) == 0 {
				lines = append(lines, "- "+reason)
				continue
			}
			for _, update := range updates {
				lines = append(lines, "- heartbeat: "+formatUpdate(update))
			}
			continue
		}
		lines = append(lines, "- "+reason)
	}
	return strings.Join(lines, "\n")
}

func (s *Service) collectUpdates(ctx context.Context, threadID modeluuid.UUID) []component.UpdateNotice {
	if s == nil || s.UpdateProvider == nil {
		return nil
	}
	feeds, err := s.UpdateProvider.UpdateFeeds(ctx)
	if err != nil {
		return []component.UpdateNotice{{Source: "heartbeat", Label: err.Error(), Kind: "error", Count: 1}}
	}
	var out []component.UpdateNotice
	for _, feed := range feeds {
		if feed == nil {
			continue
		}
		notices, err := feed.NewUpdates(ctx, component.UpdateRequest{ThreadID: threadID})
		if err != nil {
			out = append(out, component.UpdateNotice{Source: "heartbeat", Label: err.Error(), Kind: "error", Count: 1})
			continue
		}
		out = append(out, notices...)
	}
	return out
}

func intentReason(intent coremodel.TimedIntent) string {
	label := strings.TrimSpace(intent.Label)
	if label == "" {
		label = strings.TrimSpace(intent.Key)
	}
	kind := strings.TrimSpace(intent.Kind)
	if kind == "" {
		kind = "wakeup"
	}
	if label == "" || label == kind {
		return kind
	}
	return kind + ": " + label
}

func formatUpdate(update component.UpdateNotice) string {
	label := strings.TrimSpace(update.Label)
	if label == "" {
		label = strings.TrimSpace(update.Ref)
	}
	if label == "" {
		label = strings.TrimSpace(update.Source)
	}
	kind := strings.TrimSpace(update.Kind)
	if kind == "" {
		kind = "update"
	}
	count := update.Count
	if count <= 0 {
		count = 1
	}
	if source := strings.TrimSpace(update.Source); source != "" && label != source {
		return fmt.Sprintf("%s: %s (%d %s)", source, label, count, plural(kind, count))
	}
	return fmt.Sprintf("%s (%d %s)", label, count, plural(kind, count))
}

func plural(word string, count int) string {
	if count == 1 || strings.HasSuffix(word, "s") {
		return word
	}
	return word + "s"
}

func groupByTarget(intents []coremodel.TimedIntent) map[modeluuid.UUID][]coremodel.TimedIntent {
	out := map[modeluuid.UUID][]coremodel.TimedIntent{}
	for _, intent := range intents {
		if intent.TargetThreadID.IsNull() {
			continue
		}
		out[intent.TargetThreadID] = append(out[intent.TargetThreadID], intent)
	}
	for _, group := range out {
		sort.SliceStable(group, func(i, j int) bool {
			if group[i].Kind != group[j].Kind {
				return group[i].Kind < group[j].Kind
			}
			return group[i].Key < group[j].Key
		})
	}
	return out
}
