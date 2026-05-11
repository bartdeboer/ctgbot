package app_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/app"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type fakeResolver struct {
	storage    repository.Storage
	workspaces map[string]bool
}

func (r fakeResolver) ResolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error) {
	parsed, err := coremodel.ParseComponentRef(ref)
	if err != nil {
		return nil, err
	}
	registration, err := r.storage.Components().GetByTypeAndName(ctx, parsed.Type, parsed.ResolvedName())
	if err != nil {
		return nil, err
	}
	if registration == nil {
		return nil, fmt.Errorf("component not registered: %s", parsed.Ref())
	}
	return registration, nil
}

func (r fakeResolver) ResolveComponent(ctx context.Context, id modeluuid.UUID) (*component.Loaded, error) {
	registration, err := r.storage.Components().GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if registration == nil {
		return nil, fmt.Errorf("component not found: %s", id)
	}
	var impl component.Component
	switch registration.Type {
	case "source":
		impl = fakeSource{}
	case "guard":
		impl = fakeGuard{}
	case "cli":
		impl = fakeCLI{}
	case "plain":
		impl = fakeComponent{typ: "plain"}
	default:
		return nil, fmt.Errorf("unknown component type: %s", registration.Type)
	}
	return &component.Loaded{Registration: *registration, Component: impl}, nil
}

func (r fakeResolver) ValidateWorkspace(name string) error {
	if r.workspaces == nil || !r.workspaces[strings.TrimSpace(name)] {
		return fmt.Errorf("workspace not found: %s", name)
	}
	return nil
}

func (r fakeResolver) EnsureComponent(ctx context.Context, ref string, runtimeKind string, homePath string) (*coremodel.Component, error) {
	parsed, err := coremodel.ParseComponentRef(ref)
	if err != nil {
		return nil, err
	}
	runtimeKind = strings.TrimSpace(runtimeKind)
	if runtimeKind == "" {
		runtimeKind = "local"
	}
	registration, err := r.storage.Components().GetByTypeAndName(ctx, parsed.Type, parsed.ResolvedName())
	if err != nil {
		return nil, err
	}
	if registration == nil {
		registration = &coremodel.Component{
			Type:      parsed.Type,
			Name:      parsed.ResolvedName(),
			IsDefault: !parsed.ExplicitName || parsed.ResolvedName() == coremodel.DefaultComponentName(parsed.Type),
		}
	}
	registration.Runtime = runtimeKind
	registration.HomePath = strings.TrimSpace(homePath)
	registration.Enabled = true
	if err := r.storage.Components().Save(ctx, registration); err != nil {
		return nil, err
	}
	return registration, nil
}

func (r fakeResolver) Runtime(kind string) (runtimepkg.Factory, error) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		kind = "local"
	}
	return fakeRuntimeFactory{kind: kind}, nil
}

type fakeRuntimeFactory struct {
	kind string
}

func (f fakeRuntimeFactory) Kind() string {
	return f.kind
}

func (f fakeRuntimeFactory) ComponentHome(registration coremodel.Component) runtimepkg.Home {
	if strings.TrimSpace(registration.HomePath) != "" {
		return runtimepkg.Home{Path: strings.TrimSpace(registration.HomePath)}
	}
	return runtimepkg.Home{Path: "/components/" + registration.Type + "/" + registration.Name}
}

func (f fakeRuntimeFactory) RuntimeComponentHomePath(registration coremodel.Component, home runtimepkg.Home) string {
	_ = registration
	return "/runtime" + strings.TrimPrefix(home.Path, "/components")
}

func (f fakeRuntimeFactory) RuntimeWorkspacePath(workspacePath string) string {
	return strings.TrimSpace(workspacePath)
}

func (f fakeRuntimeFactory) Bind(registration coremodel.Component, home runtimepkg.Home, config runtimepkg.BindConfig) runtimepkg.Runtime {
	_, _, _ = registration, home, config
	return nil
}

type fakeComponent struct{ typ string }

func (c fakeComponent) Type() string { return c.typ }

type fakeSource struct{}

func (fakeSource) Type() string { return "source" }
func (fakeSource) RunInbound(ctx context.Context, emit component.InboundEmitter) error {
	_, _ = ctx, emit
	return nil
}
func (fakeSource) DefaultSourceExternalChatID(ctx context.Context) (string, error) {
	_ = ctx
	return "source-default", nil
}

type fakeGuard struct{}

func (fakeGuard) Type() string { return "guard" }
func (fakeGuard) HandleCompletion(ctx context.Context, request component.CompletionRequest) (*component.CompletionResult, error) {
	_, _ = ctx, request
	return nil, nil
}

type fakeCLI struct{}

type fakePingCommand struct{}

func (fakeCLI) Type() string { return "cli" }
func (fakeCLI) UsesLocalCommandRoutes() bool {
	return true
}
func (fakeCLI) CommandDefinitions() []commandengine.Definition {
	return []commandengine.Definition{{
		Pattern: "ping",
		Help:    "Ping fake CLI",
		Build: func(req *clir.Request) (any, error) {
			_ = req
			return fakePingCommand{}, nil
		},
		Sources: []commandengine.Source{commandengine.SourceCLI},
		Policy:  simplerbac.Any(simplerbac.RoleRoot),
	}}
}
func (fakeCLI) RegisterCommandHandlers(registry *commandengine.Registry) error {
	if registry == nil {
		return fmt.Errorf("missing command registry")
	}
	return commandengine.RegisterPattern[fakePingCommand](registry, "ping", func(ctx context.Context, req commandengine.Request, cmd fakePingCommand) (commandengine.Result, error) {
		_, _, _ = ctx, req, cmd
		return commandengine.Result{Text: "pong"}, nil
	})
}

func TestServiceSetStatusClearComponentGuard(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})

	source := saveComponent(t, storage, "source", "inbox")
	firstGuard := saveComponent(t, storage, "guard", "qwen")
	secondGuard := saveComponent(t, storage, "guard", "gemma")

	set, err := svc.SetComponentGuard(ctx, "source/inbox", "guard/qwen")
	if err != nil {
		t.Fatalf("SetComponentGuard() error = %v", err)
	}
	if set.Source.ID != source.ID || set.Guard.ID != firstGuard.ID || set.Binding.TargetComponentID != firstGuard.ID {
		t.Fatalf("unexpected set result: %#v", set)
	}

	status, err := svc.ComponentGuardStatus(ctx, "source/inbox")
	if err != nil {
		t.Fatalf("ComponentGuardStatus() error = %v", err)
	}
	if got, want := len(status.Bindings), 1; got != want {
		t.Fatalf("status bindings = %d, want %d", got, want)
	}
	if status.Bindings[0].GuardRef != "guard/qwen" {
		t.Fatalf("guard ref = %q, want guard/qwen", status.Bindings[0].GuardRef)
	}

	if _, err := svc.SetComponentGuard(ctx, "source/inbox", "guard/gemma"); err != nil {
		t.Fatalf("replace SetComponentGuard() error = %v", err)
	}
	bindings, err := storage.ComponentBindings().ListEnabledBySourceAndRole(ctx, source.ID, coremodel.ComponentBindingRoleGuard)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(bindings), 1; got != want {
		t.Fatalf("enabled bindings = %d, want %d", got, want)
	}
	if bindings[0].TargetComponentID != secondGuard.ID {
		t.Fatalf("enabled guard = %s, want %s", bindings[0].TargetComponentID, secondGuard.ID)
	}

	clear, err := svc.ClearComponentGuard(ctx, "source/inbox")
	if err != nil {
		t.Fatalf("ClearComponentGuard() error = %v", err)
	}
	if clear.Disabled != 1 {
		t.Fatalf("disabled = %d, want 1", clear.Disabled)
	}
	bindings, err = storage.ComponentBindings().ListEnabledBySourceAndRole(ctx, source.ID, coremodel.ComponentBindingRoleGuard)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(bindings); got != 0 {
		t.Fatalf("enabled bindings after clear = %d, want 0", got)
	}
}

func TestServiceRegisterAndListComponents(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})

	registered, err := svc.RegisterComponent(ctx, "source/inbox", "local", "/custom/source")
	if err != nil {
		t.Fatalf("RegisterComponent() error = %v", err)
	}
	if registered.Component.Ref() != "source/inbox" ||
		registered.Component.Runtime != "local" ||
		registered.Component.HomePath != "/custom/source" ||
		registered.HostHomePath != "/custom/source" ||
		registered.RuntimeHomePath != "/runtime/custom/source" {
		t.Fatalf("registered result = %#v", registered)
	}

	components, err := svc.ListComponents(ctx)
	if err != nil {
		t.Fatalf("ListComponents() error = %v", err)
	}
	if got, want := len(components), 1; got != want {
		t.Fatalf("components = %d, want %d", got, want)
	}
	info := components[0]
	if info.Component.Ref() != "source/inbox" ||
		info.RuntimeKind != "local" ||
		info.HostHomePath != "/custom/source" {
		t.Fatalf("component info = %#v", info)
	}
}

func TestServiceRunComponentCommand(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})
	saveComponent(t, storage, "cli", "test")

	help, err := svc.RunComponentCommand(ctx, app.ComponentCommandRequest{ComponentRef: "cli/test"})
	if err != nil {
		t.Fatalf("RunComponentCommand(help) error = %v", err)
	}
	if !strings.Contains(help.Text, "available component commands:") || !strings.Contains(help.Text, "cli ping") {
		t.Fatalf("help text = %q, want component command patterns", help.Text)
	}

	result, err := svc.RunComponentCommand(ctx, app.ComponentCommandRequest{
		ComponentRef: "cli/test",
		Args:         []string{"ping"},
	})
	if err != nil {
		t.Fatalf("RunComponentCommand(ping) error = %v", err)
	}
	if result.Text != "pong" {
		t.Fatalf("command text = %q, want pong", result.Text)
	}
}

func TestServiceChatManagement(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{
		storage:    storage,
		workspaces: map[string]bool{"work": true},
	})

	chat, err := svc.CreateChat(ctx, " Team ", "")
	if err != nil {
		t.Fatalf("CreateChat() error = %v", err)
	}
	if chat.Label != "Team" || !chat.Enabled {
		t.Fatalf("created chat = %#v, want trimmed enabled chat", chat)
	}

	chats, err := svc.ListChats(ctx)
	if err != nil {
		t.Fatalf("ListChats() error = %v", err)
	}
	if got, want := len(chats), 1; got != want {
		t.Fatalf("chats = %d, want %d", got, want)
	}

	updated, err := svc.SetChatWorkspace(ctx, chat.ID, "work")
	if err != nil {
		t.Fatalf("SetChatWorkspace() error = %v", err)
	}
	if updated.Workspace != "work" {
		t.Fatalf("workspace = %q, want work", updated.Workspace)
	}
	updated, err = svc.SetChatWorkspace(ctx, chat.ID, "")
	if err != nil {
		t.Fatalf("clear SetChatWorkspace() error = %v", err)
	}
	if updated.Workspace != "" {
		t.Fatalf("workspace after clear = %q, want empty", updated.Workspace)
	}

	if _, err := svc.SetChatWorkspace(ctx, chat.ID, "missing"); err == nil {
		t.Fatal("SetChatWorkspace() with unknown workspace error = nil")
	}
}

func TestServiceListInboundDropsResolvesComponentRefs(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})
	source := saveComponent(t, storage, "source", "inbox")
	lastSeen := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	if err := storage.InboundDrops().Save(ctx, &coremodel.InboundDrop{
		ComponentID:     source.ID,
		ExternalChatID:  "external-1",
		ChatLabel:       "Inbox",
		ActorLabel:      "Alice",
		ActorID:         "alice@example.com",
		LastTextPreview: "hello",
		MessageCount:    2,
		LastSeenAt:      lastSeen,
	}); err != nil {
		t.Fatal(err)
	}

	drops, err := svc.ListInboundDrops(ctx)
	if err != nil {
		t.Fatalf("ListInboundDrops() error = %v", err)
	}
	if got, want := len(drops), 1; got != want {
		t.Fatalf("drops = %d, want %d", got, want)
	}
	drop := drops[0]
	if drop.ComponentRef != "source/inbox" ||
		drop.ExternalChatID != "external-1" ||
		drop.MessageCount != 2 ||
		!drop.LastSeenAt.Equal(lastSeen) ||
		drop.ChatLabel != "Inbox" ||
		drop.ActorLabel != "Alice" ||
		drop.ActorID != "alice@example.com" ||
		drop.LastTextPreview != "hello" {
		t.Fatalf("drop info = %#v", drop)
	}
}

func TestServiceBindInboundChat(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})
	source := saveComponent(t, storage, "source", "inbox")
	if err := storage.InboundDrops().Save(ctx, &coremodel.InboundDrop{
		ComponentID:     source.ID,
		ExternalChatID:  "external-1",
		ChatLabel:       "Inbox",
		LastTextPreview: "hello",
		MessageCount:    1,
		LastSeenAt:      time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.BindInboundChat(ctx, "source/inbox", "external-1", "", "")
	if err != nil {
		t.Fatalf("BindInboundChat() error = %v", err)
	}
	if result.Chat.Label != "Inbox" || !result.Chat.Enabled {
		t.Fatalf("bound chat = %#v, want drop label and enabled chat", result.Chat)
	}
	if result.Component.ID != source.ID {
		t.Fatalf("component = %s, want %s", result.Component.ID, source.ID)
	}
	if got, want := len(result.Bindings), 1; got != want {
		t.Fatalf("bindings = %d, want %d", got, want)
	}
	binding := result.Bindings[0]
	if binding.Role != coremodel.ChatComponentRoleSource || binding.ExternalChatID != "external-1" {
		t.Fatalf("binding = %#v, want source external-1", binding)
	}
	drops, err := storage.InboundDrops().List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(drops); got != 0 {
		t.Fatalf("drops after bind = %d, want 0", got)
	}
}

func TestServiceAddAndListChatComponents(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})
	chat, err := svc.CreateChat(ctx, "Team", "")
	if err != nil {
		t.Fatalf("CreateChat() error = %v", err)
	}
	saveComponent(t, storage, "source", "inbox")

	result, err := svc.AddChatComponent(ctx, chat.ID, coremodel.ChatComponentRoleSource, "source/inbox", "")
	if err != nil {
		t.Fatalf("AddChatComponent() error = %v", err)
	}
	if result.ComponentRef != "source/inbox" || result.Runtime != "local" {
		t.Fatalf("add result component = %q runtime=%q, want source/inbox local", result.ComponentRef, result.Runtime)
	}
	if result.Binding.Role != coremodel.ChatComponentRoleSource || result.Binding.ExternalChatID != "source-default" {
		t.Fatalf("binding = %#v, want source role with default external id", result.Binding)
	}

	infos, err := svc.ListChatComponents(ctx, chat.ID)
	if err != nil {
		t.Fatalf("ListChatComponents() error = %v", err)
	}
	if got, want := len(infos), 1; got != want {
		t.Fatalf("component infos = %d, want %d", got, want)
	}
	info := infos[0]
	if info.ComponentRef != "source/inbox" ||
		info.Runtime != "local" ||
		info.Binding.Role != coremodel.ChatComponentRoleSource ||
		info.Binding.ExternalChatID != "source-default" {
		t.Fatalf("component info = %#v", info)
	}
}

func TestServiceComponentGuardValidatesCapabilities(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})

	saveComponent(t, storage, "plain", "not-source")
	saveComponent(t, storage, "source", "inbox")
	saveComponent(t, storage, "plain", "not-guard")

	_, err := svc.SetComponentGuard(ctx, "plain/not-source", "plain/not-guard")
	if err == nil || !strings.Contains(err.Error(), "does not support inbound source") {
		t.Fatalf("source validation error = %v, want inbound source error", err)
	}
	_, err = svc.SetComponentGuard(ctx, "source/inbox", "plain/not-guard")
	if err == nil || !strings.Contains(err.Error(), "does not support completion provider") {
		t.Fatalf("guard validation error = %v, want completion provider error", err)
	}
}

func saveComponent(t *testing.T, storage repository.Storage, typ string, name string) *coremodel.Component {
	t.Helper()
	registration := &coremodel.Component{Type: typ, Name: name, Runtime: "local", Enabled: true}
	if err := storage.Components().Save(context.Background(), registration); err != nil {
		t.Fatal(err)
	}
	return registration
}
