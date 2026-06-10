package timedintent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
)

func TestRunDueDeliversHeartbeatWakeWithFreshUpdates(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	threadID := modeluuid.New()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	deliverer := &fakeDeliverer{}
	service := New(storage.TimedIntents(), deliverer, fakeUpdateProvider{feed: fakeFeed{notice: component.UpdateNotice{Source: "theater", Label: "qwen-parser-lab", Kind: "message", Count: 3}}}, nil)
	service.Now = func() time.Time { return now }

	intent := coremodel.TimedIntent{
		TargetThreadID: threadID,
		OwnerThreadID:  threadID,
		Kind:           KindHeartbeat,
		Key:            KeyDefault,
		Enabled:        true,
		NextDueAt:      timePtr(now.Add(-time.Minute)),
		Every:          "30m",
		Delivery:       DeliveryTurn,
		Label:          "heartbeat",
	}
	if err := storage.TimedIntents().UpsertByTargetKindKey(ctx, &intent); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	result, err := service.RunDue(ctx)
	if err != nil {
		t.Fatalf("RunDue() error = %v", err)
	}
	if got, want := result.Delivered, 1; got != want {
		t.Fatalf("delivered = %d, want %d", got, want)
	}
	if deliverer.threadID != threadID {
		t.Fatalf("delivered thread = %s, want %s", deliverer.threadID, threadID)
	}
	if !strings.Contains(deliverer.text, "heartbeat: theater: qwen-parser-lab (3 messages)") {
		t.Fatalf("wake text = %q, want theater notice", deliverer.text)
	}
	stored, found, err := service.Heartbeat(ctx, threadID)
	if err != nil || !found {
		t.Fatalf("Heartbeat() found=%v err=%v", found, err)
	}
	if stored.NextDueAt == nil || !stored.NextDueAt.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("next due = %v, want %v", stored.NextDueAt, now.Add(30*time.Minute))
	}
}

func TestRunDueLeavesTurnWakeDueWhenThreadBusy(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	threadID := modeluuid.New()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	deliverer := &fakeDeliverer{busy: true}
	service := New(storage.TimedIntents(), deliverer, nil, nil)
	service.Now = func() time.Time { return now }

	intent := coremodel.TimedIntent{TargetThreadID: threadID, OwnerThreadID: threadID, Kind: KindWake, Key: KeyDefault, Enabled: true, NextDueAt: timePtr(now.Add(-time.Minute)), Delivery: DeliveryTurn, Label: "check build"}
	if err := storage.TimedIntents().UpsertByTargetKindKey(ctx, &intent); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	result, err := service.RunDue(ctx)
	if err != nil {
		t.Fatalf("RunDue() error = %v", err)
	}
	if got, want := result.SkippedBusy, 1; got != want {
		t.Fatalf("skipped busy = %d, want %d", got, want)
	}
	if deliverer.calls != 0 {
		t.Fatalf("deliver calls = %d, want 0", deliverer.calls)
	}
	stored, err := storage.TimedIntents().GetByTargetKindKey(ctx, threadID, KindWake, KeyDefault)
	if err != nil {
		t.Fatalf("GetByTargetKindKey() error = %v", err)
	}
	if stored.NextDueAt == nil || stored.NextDueAt.After(now) {
		t.Fatalf("next due = %v, want still due", stored.NextDueAt)
	}
}

func TestResetHeartbeatFloorMovesNextDueFromCompletedTurn(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	threadID := modeluuid.New()
	service := New(storage.TimedIntents(), nil, nil, nil)
	start := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	service.Now = func() time.Time { return start }
	if _, err := service.StartHeartbeat(ctx, threadID, "1h", coremodel.Actor{ID: "agent"}); err != nil {
		t.Fatalf("StartHeartbeat() error = %v", err)
	}
	completed := start.Add(20 * time.Minute)
	if err := service.ResetHeartbeatFloor(ctx, threadID, completed); err != nil {
		t.Fatalf("ResetHeartbeatFloor() error = %v", err)
	}
	intent, found, err := service.Heartbeat(ctx, threadID)
	if err != nil || !found {
		t.Fatalf("Heartbeat() found=%v err=%v", found, err)
	}
	want := completed.Add(time.Hour)
	if intent.NextDueAt == nil || !intent.NextDueAt.Equal(want) {
		t.Fatalf("next due = %v, want %v", intent.NextDueAt, want)
	}
}

func TestStartCronHeartbeatSchedulesNextCronTime(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	threadID := modeluuid.New()
	service := New(storage.TimedIntents(), nil, nil, nil)
	now := time.Date(2026, 6, 10, 8, 30, 0, 0, time.UTC)
	service.Now = func() time.Time { return now }

	intent, err := service.StartCronHeartbeat(ctx, threadID, "0 9-17/2 * * 1-5", "Europe/Amsterdam", "check income-growth theater", coremodel.Actor{ID: "agent"})
	if err != nil {
		t.Fatalf("StartCronHeartbeat() error = %v", err)
	}
	if got, want := intent.Cron, "0 9-17/2 * * 1-5"; got != want {
		t.Fatalf("cron = %q, want %q", got, want)
	}
	if got, want := intent.Timezone, "Europe/Amsterdam"; got != want {
		t.Fatalf("timezone = %q, want %q", got, want)
	}
	if got, want := intent.Label, "check income-growth theater"; got != want {
		t.Fatalf("label = %q, want %q", got, want)
	}
	want := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC) // 11:00 Europe/Amsterdam.
	if intent.NextDueAt == nil || !intent.NextDueAt.Equal(want) {
		t.Fatalf("next due = %v, want %v", intent.NextDueAt, want)
	}
}

func TestRunDueAdvancesCronHeartbeatToNextCronTime(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	threadID := modeluuid.New()
	now := time.Date(2026, 6, 10, 9, 0, 0, 0, time.UTC)
	deliverer := &fakeDeliverer{}
	service := New(storage.TimedIntents(), deliverer, nil, nil)
	service.Now = func() time.Time { return now }

	intent := coremodel.TimedIntent{
		TargetThreadID: threadID,
		OwnerThreadID:  threadID,
		Kind:           KindHeartbeat,
		Key:            KeyDefault,
		Enabled:        true,
		NextDueAt:      timePtr(now.Add(-time.Minute)),
		Cron:           "0 9-17/2 * * 1-5",
		Timezone:       "Europe/Amsterdam",
		Delivery:       DeliveryTurn,
		Label:          "check income-growth theater",
	}
	if err := storage.TimedIntents().UpsertByTargetKindKey(ctx, &intent); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}

	result, err := service.RunDue(ctx)
	if err != nil {
		t.Fatalf("RunDue() error = %v", err)
	}
	if got, want := result.Delivered, 1; got != want {
		t.Fatalf("delivered = %d, want %d", got, want)
	}
	if !strings.Contains(deliverer.text, "heartbeat: check income-growth theater") {
		t.Fatalf("wake text = %q, want heartbeat reason", deliverer.text)
	}
	stored, found, err := service.Heartbeat(ctx, threadID)
	if err != nil || !found {
		t.Fatalf("Heartbeat() found=%v err=%v", found, err)
	}
	want := time.Date(2026, 6, 10, 11, 0, 0, 0, time.UTC) // 13:00 Europe/Amsterdam.
	if stored.NextDueAt == nil || !stored.NextDueAt.Equal(want) {
		t.Fatalf("next due = %v, want %v", stored.NextDueAt, want)
	}
}

func TestRunDueExpiresIntentBeforeDelivery(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	threadID := modeluuid.New()
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	deliverer := &fakeDeliverer{}
	service := New(storage.TimedIntents(), deliverer, nil, nil)
	service.Now = func() time.Time { return now }

	intent := coremodel.TimedIntent{
		TargetThreadID: threadID,
		OwnerThreadID:  threadID,
		Kind:           KindWake,
		Key:            KeyDefault,
		Enabled:        true,
		NextDueAt:      timePtr(now.Add(-time.Minute)),
		ExpiresAt:      timePtr(now.Add(-time.Second)),
		Delivery:       DeliveryTurn,
		Label:          "too late",
	}
	if err := storage.TimedIntents().UpsertByTargetKindKey(ctx, &intent); err != nil {
		t.Fatalf("Upsert() error = %v", err)
	}
	result, err := service.RunDue(ctx)
	if err != nil {
		t.Fatalf("RunDue() error = %v", err)
	}
	if got, want := result.Expired, 1; got != want {
		t.Fatalf("expired = %d, want %d", got, want)
	}
	if deliverer.calls != 0 {
		t.Fatalf("deliver calls = %d, want 0", deliverer.calls)
	}
	stored, err := storage.TimedIntents().GetByTargetKindKey(ctx, threadID, KindWake, KeyDefault)
	if err != nil {
		t.Fatalf("GetByTargetKindKey() error = %v", err)
	}
	if stored.Enabled || stored.LastStatus != coremodel.TimedIntentStatusExpired {
		t.Fatalf("stored = %#v, want disabled expired", stored)
	}
}

func timePtr(t time.Time) *time.Time { return &t }

type fakeDeliverer struct {
	busy     bool
	calls    int
	threadID modeluuid.UUID
	text     string
}

func (f *fakeDeliverer) ThreadBusy(threadID modeluuid.UUID) bool { return f.busy }
func (f *fakeDeliverer) DeliverWake(ctx context.Context, threadID modeluuid.UUID, text string) error {
	_ = ctx
	f.calls++
	f.threadID = threadID
	f.text = text
	return nil
}

type fakeUpdateProvider struct{ feed component.UpdateFeed }

func (f fakeUpdateProvider) UpdateFeeds(ctx context.Context) ([]component.UpdateFeed, error) {
	_ = ctx
	return []component.UpdateFeed{f.feed}, nil
}

type fakeFeed struct{ notice component.UpdateNotice }

func (f fakeFeed) NewUpdates(ctx context.Context, req component.UpdateRequest) ([]component.UpdateNotice, error) {
	_ = ctx
	if req.ThreadID.IsNull() {
		return nil, nil
	}
	return []component.UpdateNotice{f.notice}, nil
}
