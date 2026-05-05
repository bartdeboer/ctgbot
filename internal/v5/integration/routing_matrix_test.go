package integration

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/messenger"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	v5broker "github.com/bartdeboer/ctgbot/internal/v5/broker"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
	v5system "github.com/bartdeboer/ctgbot/internal/v5/system"
)

func TestV5RoutingMatrixProfilesChatsThreadsAndContinuity(t *testing.T) {
	withTempCwd(t, func(root string) {
		ctx := context.Background()

		storage := newSQLiteStorage(t)
		runtimeState := &runtimeState{}

		gmailStates := map[string]*multiEventSourceState{
			"mockgmail/work": {
				events: []component.InboundEvent{
					{
						ExternalID: "alpha-1",
						Payload: messenger.InboundPayload{
							ProviderType:      "mockgmail",
							ProviderChatID:    "gmail-alpha",
							ProviderThreadID:  "shared-thread",
							ProviderMessageID: "alpha-1",
							Actor:             actorWithRoles("", "alpha@example.com"),
							Text:              messenger.TextMessage{Text: "alpha one"},
						},
					},
					{
						ExternalID: "alpha-2",
						Payload: messenger.InboundPayload{
							ProviderType:      "mockgmail",
							ProviderChatID:    "gmail-alpha",
							ProviderThreadID:  "shared-thread",
							ProviderMessageID: "alpha-2",
							Actor:             actorWithRoles("", "alpha@example.com"),
							Text:              messenger.TextMessage{Text: "alpha two"},
						},
					},
				},
			},
			"mockgmail/personal": {
				events: []component.InboundEvent{
					{
						ExternalID: "beta-1",
						Payload: messenger.InboundPayload{
							ProviderType:      "mockgmail",
							ProviderChatID:    "gmail-beta",
							ProviderThreadID:  "shared-thread",
							ProviderMessageID: "beta-1",
							Actor:             actorWithRoles("", "beta@example.com"),
							Text:              messenger.TextMessage{Text: "beta one"},
						},
					},
					{
						ExternalID: "beta-2",
						Payload: messenger.InboundPayload{
							ProviderType:      "mockgmail",
							ProviderChatID:    "gmail-beta",
							ProviderThreadID:  "beta-other-thread",
							ProviderMessageID: "beta-2",
							Actor:             actorWithRoles("", "beta@example.com"),
							Text:              messenger.TextMessage{Text: "beta other"},
						},
					},
				},
			},
		}
		relayStates := map[string]*relayState{
			"mockrelay/alpha": {},
			"mockrelay/beta":  {},
		}
		agentStates := map[string]*agentState{
			"mockagent/work":     {},
			"mockagent/personal": {},
		}

		registry := component.NewRegistry()
		if err := registry.Add("mockgmail", func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
			_, _, _, _, _ = ctx, runtime, home, storage, registration
			state, ok := gmailStates[registration.Ref()]
			if !ok {
				return nil, fmt.Errorf("missing gmail state for %s", registration.Ref())
			}
			return &multiEventSource{
				componentID: registration.ID,
				state:       state,
			}, nil
		}); err != nil {
			t.Fatalf("register mockgmail: %v", err)
		}
		if err := registry.Add("mockrelay", func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
			_, _, _, _, _ = ctx, runtime, home, storage, registration
			state, ok := relayStates[registration.Ref()]
			if !ok {
				return nil, fmt.Errorf("missing relay state for %s", registration.Ref())
			}
			return &mockRelay{state: state}, nil
		}); err != nil {
			t.Fatalf("register mockrelay: %v", err)
		}
		if err := registry.Add("mockagent", func(ctx context.Context, registration coremodel.Component, runtime v5runtime.Factory, home v5runtime.Home, storage repository.Storage) (component.Component, error) {
			_, _, _ = ctx, home, storage
			state, ok := agentStates[registration.Ref()]
			if !ok {
				return nil, fmt.Errorf("missing agent state for %s", registration.Ref())
			}
			return &mockAgent{
				componentID: registration.ID,
				runtime:     runtime.Bind(registration, home, "", nil),
				state:       state,
			}, nil
		}); err != nil {
			t.Fatalf("register mockagent: %v", err)
		}

		system := v5system.New(storage, map[string]v5system.Workspace{
			"work":     {Name: "work", Path: filepath.Join(root, "workspaces", "work-root")},
			"personal": {Name: "personal", Path: filepath.Join(root, "workspaces", "personal-root")},
		}, map[string]v5runtime.Factory{
			"local": fakeRuntimeFactory{
				runtimeKind:    "local",
				rootDir:        root,
				componentsRoot: filepath.Join(root, ".ctgbot", "components"),
				state:          runtimeState,
			},
		}, registry)
		system.StateRoot = filepath.Join(root, ".ctgbot")

		workGmail, err := system.EnsureComponent(ctx, "mockgmail/work", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockgmail/work) error = %v", err)
		}
		personalGmail, err := system.EnsureComponent(ctx, "mockgmail/personal", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockgmail/personal) error = %v", err)
		}
		alphaRelay, err := system.EnsureComponent(ctx, "mockrelay/alpha", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockrelay/alpha) error = %v", err)
		}
		betaRelay, err := system.EnsureComponent(ctx, "mockrelay/beta", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockrelay/beta) error = %v", err)
		}
		workAgent, err := system.EnsureComponent(ctx, "mockagent/work", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockagent/work) error = %v", err)
		}
		personalAgent, err := system.EnsureComponent(ctx, "mockagent/personal", "local", "")
		if err != nil {
			t.Fatalf("EnsureComponent(mockagent/personal) error = %v", err)
		}

		if err := system.AuthComponent(ctx, workAgent.Ref(), "", "", 0, 0, nil, nil); err != nil {
			t.Fatalf("AuthComponent(work agent) error = %v", err)
		}
		if err := system.AuthComponent(ctx, personalAgent.Ref(), "", "", 0, 0, nil, nil); err != nil {
			t.Fatalf("AuthComponent(personal agent) error = %v", err)
		}

		alpha := &coremodel.Chat{Label: "alpha", Workspace: "work", Enabled: true}
		beta := &coremodel.Chat{Label: "beta", Workspace: "personal", Enabled: true}
		if err := storage.Chats().Save(ctx, alpha); err != nil {
			t.Fatalf("Chats().Save(alpha) error = %v", err)
		}
		if err := storage.Chats().Save(ctx, beta); err != nil {
			t.Fatalf("Chats().Save(beta) error = %v", err)
		}

		mustBindChatComponent(t, ctx, system, alpha.ID, coremodel.ChatComponentRoleSource, workGmail.Ref(), "gmail-alpha")
		mustBindChatComponent(t, ctx, system, alpha.ID, coremodel.ChatComponentRoleRelay, alphaRelay.Ref(), "telegram-alpha")
		mustBindChatComponent(t, ctx, system, alpha.ID, coremodel.ChatComponentRoleAgent, workAgent.Ref(), "")

		mustBindChatComponent(t, ctx, system, beta.ID, coremodel.ChatComponentRoleSource, personalGmail.Ref(), "gmail-beta")
		mustBindChatComponent(t, ctx, system, beta.ID, coremodel.ChatComponentRoleRelay, betaRelay.Ref(), "telegram-beta")
		mustBindChatComponent(t, ctx, system, beta.ID, coremodel.ChatComponentRoleAgent, personalAgent.Ref(), "")

		b := v5broker.New(storage, system, nil)
		if err := b.Run(ctx); err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if gmailStates["mockgmail/work"].runCalls != 1 {
			t.Fatalf("work source run calls = %d, want 1", gmailStates["mockgmail/work"].runCalls)
		}
		if gmailStates["mockgmail/personal"].runCalls != 1 {
			t.Fatalf("personal source run calls = %d, want 1", gmailStates["mockgmail/personal"].runCalls)
		}
		if agentStates["mockagent/work"].turnCalls != 2 {
			t.Fatalf("work agent turn calls = %d, want 2", agentStates["mockagent/work"].turnCalls)
		}
		if agentStates["mockagent/personal"].turnCalls != 2 {
			t.Fatalf("personal agent turn calls = %d, want 2", agentStates["mockagent/personal"].turnCalls)
		}

		if got := len(runtimeState.execs); got != 4 {
			t.Fatalf("runtime exec record count = %d, want 4", got)
		}

		recordAlphaOne := findExecRecord(t, runtimeState.execs, "reply alpha one")
		recordAlphaTwo := findExecRecord(t, runtimeState.execs, "reply alpha two")
		recordBetaOne := findExecRecord(t, runtimeState.execs, "reply beta one")
		recordBetaOther := findExecRecord(t, runtimeState.execs, "reply beta other")

		if recordAlphaOne.ThreadID != recordAlphaTwo.ThreadID {
			t.Fatalf("alpha thread continuity broken: %s != %s", recordAlphaOne.ThreadID, recordAlphaTwo.ThreadID)
		}
		if recordAlphaOne.ThreadID == recordBetaOne.ThreadID {
			t.Fatalf("shared provider thread id leaked across chats: alpha=%s beta=%s", recordAlphaOne.ThreadID, recordBetaOne.ThreadID)
		}
		if recordBetaOne.ThreadID == recordBetaOther.ThreadID {
			t.Fatalf("beta distinct provider threads collapsed to one internal thread: %s", recordBetaOne.ThreadID)
		}

		workHome := filepath.Join(root, ".ctgbot", "components", "mockagent", "work")
		personalHome := filepath.Join(root, ".ctgbot", "components", "mockagent", "personal")
		if _, err := os.Stat(filepath.Join(workHome, "auth.json")); err != nil {
			t.Fatalf("missing work auth file: %v", err)
		}
		if _, err := os.Stat(filepath.Join(personalHome, "auth.json")); err != nil {
			t.Fatalf("missing personal auth file: %v", err)
		}
		if recordAlphaOne.HomeHostPath != workHome || recordAlphaTwo.HomeHostPath != workHome {
			t.Fatalf("work agent home mismatch: alpha one=%s alpha two=%s want %s", recordAlphaOne.HomeHostPath, recordAlphaTwo.HomeHostPath, workHome)
		}
		if recordBetaOne.HomeHostPath != personalHome || recordBetaOther.HomeHostPath != personalHome {
			t.Fatalf("personal agent home mismatch: beta one=%s beta other=%s want %s", recordBetaOne.HomeHostPath, recordBetaOther.HomeHostPath, personalHome)
		}
		if got, want := recordAlphaOne.Workspace, filepath.Join(root, "workspaces", "work-root"); got != want {
			t.Fatalf("alpha workspace = %s, want %s", got, want)
		}
		if got, want := recordBetaOne.Workspace, filepath.Join(root, "workspaces", "personal-root"); got != want {
			t.Fatalf("beta workspace = %s, want %s", got, want)
		}

		if len(relayStates["mockrelay/alpha"].payloads) != 2 {
			t.Fatalf("alpha relay payload count = %d, want 2", len(relayStates["mockrelay/alpha"].payloads))
		}
		if len(relayStates["mockrelay/beta"].payloads) != 2 {
			t.Fatalf("beta relay payload count = %d, want 2", len(relayStates["mockrelay/beta"].payloads))
		}
		assertRelayTargets(t, relayStates["mockrelay/alpha"].payloads, "telegram-alpha")
		assertRelayTargets(t, relayStates["mockrelay/beta"].payloads, "telegram-beta")
		assertRelayTexts(t, relayStates["mockrelay/alpha"].payloads, []string{"done", "done"})
		assertRelayTexts(t, relayStates["mockrelay/beta"].payloads, []string{"done", "done"})
	})
}

type multiEventSourceState struct {
	runCalls int
	events   []component.InboundEvent
}

type multiEventSource struct {
	componentID modeluuid.UUID
	state       *multiEventSourceState
}

func (m *multiEventSource) Type() string {
	return "mockgmail"
}

func (m *multiEventSource) RunInbound(ctx context.Context, emit component.InboundEmitter) error {
	m.state.runCalls++
	for _, event := range m.state.events {
		event.ComponentID = m.componentID
		if err := emit(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func mustBindChatComponent(t *testing.T, ctx context.Context, system *v5system.System, chatID modeluuid.UUID, role coremodel.ChatComponentRole, ref string, externalChatID string) {
	t.Helper()
	if _, err := system.BindChatComponent(ctx, chatID, role, ref, externalChatID); err != nil {
		t.Fatalf("BindChatComponent(%s, %s) error = %v", role, ref, err)
	}
}

func findExecRecord(t *testing.T, records []execRecord, want string) execRecord {
	t.Helper()
	for _, record := range records {
		if strings.Join(record.Args, " ") == want {
			return record
		}
	}
	t.Fatalf("exec record %q not found in %#v", want, records)
	return execRecord{}
}

func assertRelayTargets(t *testing.T, payloads []messenger.OutboundPayload, wantChatID string) {
	t.Helper()
	for _, payload := range payloads {
		if payload.ProviderChatID != wantChatID {
			t.Fatalf("relay provider chat id = %q, want %q", payload.ProviderChatID, wantChatID)
		}
		if payload.ProviderThreadID != "" {
			t.Fatalf("relay provider thread id = %q, want empty fallback", payload.ProviderThreadID)
		}
		if strings.HasPrefix(payload.ProviderChatID, "gmail-") {
			t.Fatalf("relay unexpectedly targeted gmail inbox: %#v", payload)
		}
	}
}

func assertRelayTexts(t *testing.T, payloads []messenger.OutboundPayload, want []string) {
	t.Helper()
	if len(payloads) != len(want) {
		t.Fatalf("relay payload count = %d, want %d", len(payloads), len(want))
	}
	for i, payload := range payloads {
		if payload.Text.Text != want[i] {
			t.Fatalf("relay text at %d = %q, want %q", i, payload.Text.Text, want[i])
		}
	}
}
