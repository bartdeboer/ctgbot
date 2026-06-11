package timedintent

import (
	"context"
	"errors"
	"fmt"
	"regexp"
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

func (s *Service) StartCronHeartbeat(ctx context.Context, threadID modeluuid.UUID, expr string, timezone string, label string, owner coremodel.Actor) (*coremodel.TimedIntent, error) {
	if s == nil || s.Intents == nil {
		return nil, fmt.Errorf("missing timed intent repository")
	}
	if threadID.IsNull() {
		return nil, fmt.Errorf("missing thread id")
	}
	expr = strings.TrimSpace(expr)
	timezone = strings.TrimSpace(timezone)
	next, err := nextCronDue(expr, timezone, s.now())
	if err != nil {
		return nil, fmt.Errorf("parse heartbeat cron: %w", err)
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = "heartbeat"
	}
	intent := coremodel.TimedIntent{
		TargetThreadID: threadID,
		OwnerThreadID:  threadID,
		OwnerActorID:   strings.TrimSpace(owner.ID),
		Kind:           KindHeartbeat,
		Key:            KeyDefault,
		Enabled:        true,
		NextDueAt:      &next,
		Cron:           expr,
		Timezone:       timezone,
		Delivery:       DeliveryTurn,
		Label:          label,
		LastStatus:     coremodel.TimedIntentStatusNever,
	}
	if err := s.Intents.UpsertByTargetKindKey(ctx, &intent); err != nil {
		return nil, err
	}
	return &intent, nil
}

func (s *Service) ScheduleWakeOnce(ctx context.Context, threadID modeluuid.UUID, delay string, label string, owner coremodel.Actor) (*coremodel.TimedIntent, error) {
	if s == nil || s.Intents == nil {
		return nil, fmt.Errorf("missing timed intent repository")
	}
	if threadID.IsNull() {
		return nil, fmt.Errorf("missing thread id")
	}
	delay = strings.TrimSpace(delay)
	duration, err := time.ParseDuration(delay)
	if err != nil {
		return nil, fmt.Errorf("parse wake delay: %w", err)
	}
	if duration <= 0 {
		return nil, fmt.Errorf("wake delay must be positive")
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, fmt.Errorf("missing wake reason")
	}
	next := s.now().Add(duration)
	intent := coremodel.TimedIntent{
		TargetThreadID: threadID,
		OwnerThreadID:  threadID,
		OwnerActorID:   strings.TrimSpace(owner.ID),
		Kind:           KindWake,
		Key:            KeyDefault,
		Enabled:        true,
		NextDueAt:      &next,
		Delivery:       DeliveryTurn,
		Label:          label,
		LastStatus:     coremodel.TimedIntentStatusNever,
	}
	if err := s.Intents.UpsertByTargetKindKey(ctx, &intent); err != nil {
		return nil, err
	}
	return &intent, nil
}

func (s *Service) ScheduleWakeCron(ctx context.Context, threadID modeluuid.UUID, expr string, timezone string, label string, owner coremodel.Actor) (*coremodel.TimedIntent, error) {
	if s == nil || s.Intents == nil {
		return nil, fmt.Errorf("missing timed intent repository")
	}
	if threadID.IsNull() {
		return nil, fmt.Errorf("missing thread id")
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return nil, fmt.Errorf("missing scheduled wake reason")
	}
	expr = strings.TrimSpace(expr)
	timezone = strings.TrimSpace(timezone)
	next, err := nextCronDue(expr, timezone, s.now())
	if err != nil {
		return nil, fmt.Errorf("parse scheduled wake cron: %w", err)
	}
	intent := coremodel.TimedIntent{
		TargetThreadID: threadID,
		OwnerThreadID:  threadID,
		OwnerActorID:   strings.TrimSpace(owner.ID),
		Kind:           KindCron,
		Key:            keyFromLabel(label),
		Enabled:        true,
		NextDueAt:      &next,
		Cron:           expr,
		Timezone:       timezone,
		Delivery:       DeliveryTurn,
		Label:          label,
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

func (s *Service) ClearWakeOnce(ctx context.Context, threadID modeluuid.UUID) (bool, error) {
	if s == nil || s.Intents == nil {
		return false, fmt.Errorf("missing timed intent repository")
	}
	if threadID.IsNull() {
		return false, fmt.Errorf("missing thread id")
	}
	return s.Intents.DeleteByTargetKindKey(ctx, threadID, KindWake, KeyDefault)
}

func (s *Service) ClearScheduledWake(ctx context.Context, threadID modeluuid.UUID, label string) (bool, error) {
	if s == nil || s.Intents == nil {
		return false, fmt.Errorf("missing timed intent repository")
	}
	if threadID.IsNull() {
		return false, fmt.Errorf("missing thread id")
	}
	return s.Intents.DeleteByTargetKindKey(ctx, threadID, KindCron, keyFromLabel(label))
}

func (s *Service) ClearAllScheduledWakes(ctx context.Context, threadID modeluuid.UUID) (int, error) {
	if s == nil || s.Intents == nil {
		return 0, fmt.Errorf("missing timed intent repository")
	}
	if threadID.IsNull() {
		return 0, fmt.Errorf("missing thread id")
	}
	intents, err := s.Intents.ListByTarget(ctx, threadID)
	if err != nil {
		return 0, err
	}
	var removed int
	for _, intent := range intents {
		if intent.Kind != KindCron {
			continue
		}
		deleted, err := s.Intents.DeleteByTargetKindKey(ctx, threadID, KindCron, intent.Key)
		if err != nil {
			return removed, err
		}
		if deleted {
			removed++
		}
	}
	return removed, nil
}

func (s *Service) ThreadWakes(ctx context.Context, threadID modeluuid.UUID) ([]coremodel.TimedIntent, error) {
	if s == nil || s.Intents == nil {
		return nil, fmt.Errorf("missing timed intent repository")
	}
	if threadID.IsNull() {
		return nil, fmt.Errorf("missing thread id")
	}
	intents, err := s.Intents.ListByTarget(ctx, threadID)
	if err != nil {
		return nil, err
	}
	out := make([]coremodel.TimedIntent, 0, len(intents))
	for _, intent := range intents {
		switch intent.Kind {
		case KindHeartbeat, KindWake, KindCron:
			out = append(out, intent)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		left, right := out[i], out[j]
		if left.NextDueAt == nil && right.NextDueAt != nil {
			return false
		}
		if left.NextDueAt != nil && right.NextDueAt == nil {
			return true
		}
		if left.NextDueAt != nil && right.NextDueAt != nil && !left.NextDueAt.Equal(*right.NextDueAt) {
			return left.NextDueAt.Before(*right.NextDueAt)
		}
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		return left.Key < right.Key
	})
	return out, nil
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
	if now.IsZero() {
		now = s.now()
	}
	// Heartbeat reset means "this thread received attention." Interval
	// heartbeats move by idle duration; cron heartbeats move to the next
	// calendar slot. One-shot and scheduled wakes are deliberately untouched.
	next, err := nextHeartbeatDue(*intent, now)
	if err != nil {
		return err
	}
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
		if intent.Kind == KindWake {
			_, err := s.Intents.DeleteByTargetKindKey(ctx, intent.TargetThreadID, intent.Kind, intent.Key)
			return err
		}
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
	cronExpr := strings.TrimSpace(intent.Cron)
	if cronExpr != "" {
		next, err := nextCronDue(cronExpr, intent.Timezone, now)
		if err != nil {
			return nil, false, err
		}
		return &next, false, nil
	}
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

func nextHeartbeatDue(intent coremodel.TimedIntent, now time.Time) (time.Time, error) {
	if cronExpr := strings.TrimSpace(intent.Cron); cronExpr != "" {
		return nextCronDue(cronExpr, intent.Timezone, now)
	}
	every, err := time.ParseDuration(strings.TrimSpace(intent.Every))
	if err != nil {
		return time.Time{}, err
	}
	if every <= 0 {
		return time.Time{}, fmt.Errorf("heartbeat interval must be positive")
	}
	return now.UTC().Add(every), nil
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
	if kind == KindCron {
		kind = "scheduled"
	}
	if kind == KindWake {
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

var keyPartRE = regexp.MustCompile(`[^a-z0-9]+`)

func keyFromLabel(label string) string {
	key := strings.ToLower(strings.TrimSpace(label))
	key = keyPartRE.ReplaceAllString(key, "-")
	key = strings.Trim(key, "-")
	if key == "" {
		return KeyDefault
	}
	if len(key) > 80 {
		key = strings.Trim(key[:80], "-")
	}
	if key == "" {
		return KeyDefault
	}
	return key
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
