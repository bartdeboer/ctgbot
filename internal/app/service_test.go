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
	"github.com/bartdeboer/ctgbot/internal/inbound"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
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
	case "filters":
		impl = fakeFilter{}
	case "cli":
		impl = fakeCLI{}
	case "image":
		impl = fakeImageProvider{ref: registration.Ref()}
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
func (fakeSource) DefaultSourceExternalChannelID(ctx context.Context) (string, error) {
	_ = ctx
	return "source-default", nil
}

type fakeFilter struct{}

func (fakeFilter) InboundFilterPrecedence() int { return 10000 }
func (fakeFilter) Type() string                 { return "filters" }
func (fakeFilter) FilterInbound(ctx context.Context, input inbound.ChannelEvent) (inbound.FilterResult, error) {
	_ = ctx
	return inbound.Drop(input, "fake-filter"), nil
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

type fakeImageProvider struct{ ref string }

func (f fakeImageProvider) Type() string { return "image" }
func (f fakeImageProvider) RuntimeImageTargets(ctx context.Context) ([]runtimeimage.Target, error) {
	_ = ctx
	return []runtimeimage.Target{{
		Name:       "fake",
		Ref:        f.ref,
		Image:      "ctgbot-fake:latest",
		Dockerfile: "fake.Dockerfile",
	}}, nil
}

func TestServiceAddListRemoveClearChatComponentFilters(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})

	chat := saveChat(t, storage, "Team")
	source := saveComponent(t, storage, "source", "inbox")
	firstFilter := saveComponent(t, storage, "filters", "allowlist")
	secondFilter := saveComponent(t, storage, "filters", "other")
	sourceBinding := saveChatComponent(t, storage, chat.ID, source.ID, coremodel.ChatComponentRoleSource, "inbox")

	add, err := svc.AddChatComponentFilter(ctx, chat.ID.String(), "source/inbox", "", "filters/allowlist")
	if err != nil {
		t.Fatalf("AddChatComponentFilter() error = %v", err)
	}
	if add.Source.ID != source.ID || add.Filter.ID != firstFilter.ID || add.SourceBinding.ID != sourceBinding.ID || add.Binding.FilterComponentID != firstFilter.ID {
		t.Fatalf("unexpected add result: %#v", add)
	}

	list, err := svc.ListChatComponentFilters(ctx, chat.ID.String(), "source/inbox", "")
	if err != nil {
		t.Fatalf("ListChatComponentFilters() error = %v", err)
	}
	if got, want := len(list.Bindings), 1; got != want {
		t.Fatalf("list bindings = %d, want %d", got, want)
	}
	if list.Bindings[0].FilterRef != "filters/allowlist" {
		t.Fatalf("filter ref = %q, want filters/allowlist", list.Bindings[0].FilterRef)
	}

	if _, err := svc.AddChatComponentFilter(ctx, chat.ID.String(), "source/inbox", "", "filters/other"); err != nil {
		t.Fatalf("second AddChatComponentFilter() error = %v", err)
	}
	bindings, err := storage.InboundFilterBindings().ListEnabledBySourceBindingID(ctx, sourceBinding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(bindings), 2; got != want {
		t.Fatalf("enabled bindings = %d, want %d", got, want)
	}
	remove, err := svc.RemoveChatComponentFilter(ctx, chat.ID.String(), "source/inbox", "", "filters/other")
	if err != nil {
		t.Fatalf("RemoveChatComponentFilter() error = %v", err)
	}
	if !remove.Disabled || remove.Filter.ID != secondFilter.ID {
		t.Fatalf("remove result = %#v, want disabled second filter", remove)
	}
	bindings, err = storage.InboundFilterBindings().ListEnabledBySourceBindingID(ctx, sourceBinding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := len(bindings), 1; got != want {
		t.Fatalf("enabled bindings after remove = %d, want %d", got, want)
	}
	if bindings[0].FilterComponentID != firstFilter.ID {
		t.Fatalf("remaining filter = %s, want %s", bindings[0].FilterComponentID, firstFilter.ID)
	}

	clear, err := svc.ClearChatComponentFilters(ctx, chat.ID.String(), "source/inbox", "")
	if err != nil {
		t.Fatalf("ClearChatComponentFilters() error = %v", err)
	}
	if clear.Disabled != 1 {
		t.Fatalf("disabled = %d, want 1", clear.Disabled)
	}
	bindings, err = storage.InboundFilterBindings().ListEnabledBySourceBindingID(ctx, sourceBinding.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(bindings); got != 0 {
		t.Fatalf("enabled bindings after clear = %d, want 0", got)
	}
}

func TestServiceComponentFilterRequiresExternalChannelForAmbiguousSourceBindings(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})
	chat := saveChat(t, storage, "Team")
	source := saveComponent(t, storage, "source", "inbox")
	filter := saveComponent(t, storage, "filters", "allowlist")
	first := saveChatComponent(t, storage, chat.ID, source.ID, coremodel.ChatComponentRoleSource, "inbox-a")
	second := saveChatComponent(t, storage, chat.ID, source.ID, coremodel.ChatComponentRoleSource, "inbox-b")

	_, err := svc.AddChatComponentFilter(ctx, chat.ID.String(), "source/inbox", "", "filters/allowlist")
	if err == nil || !strings.Contains(err.Error(), "specify --external-channel-id") {
		t.Fatalf("ambiguous error = %v, want external-channel-id hint", err)
	}
	add, err := svc.AddChatComponentFilter(ctx, chat.ID.String(), "source/inbox", "inbox-b", "filters/allowlist")
	if err != nil {
		t.Fatalf("AddChatComponentFilter(explicit) error = %v", err)
	}
	if add.Filter.ID != filter.ID || add.SourceBinding.ID != second.ID || add.SourceBinding.ID == first.ID {
		t.Fatalf("add result = %#v, want second binding", add)
	}
}

func TestServiceComponentFilterValidatesCapabilities(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})
	chat := saveChat(t, storage, "Team")

	saveComponent(t, storage, "plain", "not-source")
	source := saveComponent(t, storage, "source", "inbox")
	saveComponent(t, storage, "plain", "not-filter")
	saveChatComponent(t, storage, chat.ID, source.ID, coremodel.ChatComponentRoleSource, "inbox")

	_, err := svc.AddChatComponentFilter(ctx, chat.ID.String(), "plain/not-source", "", "plain/not-filter")
	if err == nil || !strings.Contains(err.Error(), "does not support inbound source") {
		t.Fatalf("source validation error = %v, want inbound source error", err)
	}
	_, err = svc.AddChatComponentFilter(ctx, chat.ID.String(), "source/inbox", "", "plain/not-filter")
	if err == nil || !strings.Contains(err.Error(), "does not support inbound event filtering") {
		t.Fatalf("filter validation error = %v, want inbound event filtering error", err)
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
	if chats[0].Chat.ID != chat.ID {
		t.Fatalf("chat list ID = %s, want %s", chats[0].Chat.ID, chat.ID)
	}
	if chats[0].ShortID == "" || !strings.HasSuffix(chat.ID.String(), chats[0].ShortID) {
		t.Fatalf("chat short ID = %q, want suffix of %s", chats[0].ShortID, chat.ID)
	}
	resolvedFull, err := svc.ResolveChatRef(ctx, chat.ID.String())
	if err != nil {
		t.Fatalf("ResolveChatRef(full) error = %v", err)
	}
	if resolvedFull != chat.ID {
		t.Fatalf("resolved full = %s, want %s", resolvedFull, chat.ID)
	}
	resolvedShort, err := svc.ResolveChatRef(ctx, chats[0].ShortID)
	if err != nil {
		t.Fatalf("ResolveChatRef(short) error = %v", err)
	}
	if resolvedShort != chat.ID {
		t.Fatalf("resolved short = %s, want %s", resolvedShort, chat.ID)
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

func TestServiceResolveChatRefErrors(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})
	first := fixedChatUUID(1)
	second := fixedChatUUID(2)
	for _, chat := range []*coremodel.Chat{
		{ID: first, Label: "first", Enabled: true},
		{ID: second, Label: "second", Enabled: true},
	} {
		if err := storage.Chats().Save(ctx, chat); err != nil {
			t.Fatalf("Save(chat) error = %v", err)
		}
	}

	_, err := svc.ResolveChatRef(ctx, "missing")
	if err == nil || !strings.Contains(err.Error(), "chat not found: missing") {
		t.Fatalf("ResolveChatRef(missing) error = %v, want chat not found", err)
	}

	_, err = svc.ResolveChatRef(ctx, "0")
	if err == nil {
		t.Fatal("ResolveChatRef(ambiguous) error = nil")
	}
	for _, want := range []string{
		"chat id 0 is ambiguous",
		"candidates:",
		first.String(),
		"first",
		second.String(),
		"second",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("ambiguous error missing %q:\n%s", want, err)
		}
	}
}

func fixedChatUUID(last byte) modeluuid.UUID {
	var id modeluuid.UUID
	id[6] = last
	return id
}

func TestServiceListInboundDropsResolvesComponentRefs(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})
	source := saveComponent(t, storage, "source", "inbox")
	lastSeen := time.Date(2026, 5, 11, 12, 0, 0, 0, time.UTC)
	if err := storage.InboundDrops().Save(ctx, &coremodel.InboundDrop{
		ComponentID:       source.ID,
		ExternalChannelID: "external-1",
		ChatLabel:         "Inbox",
		ActorLabel:        "Alice",
		ActorID:           "alice@example.com",
		LastTextPreview:   "hello",
		MessageCount:      2,
		LastSeenAt:        lastSeen,
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
		drop.ExternalChannelID != "external-1" ||
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
		ComponentID:       source.ID,
		ExternalChannelID: "external-1",
		ChatLabel:         "Inbox",
		LastTextPreview:   "hello",
		MessageCount:      1,
		LastSeenAt:        time.Now(),
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
	if binding.Role != coremodel.ChatComponentRoleSource || binding.ExternalChannelID != "external-1" {
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
	if result.Binding.Role != coremodel.ChatComponentRoleSource || result.Binding.ExternalChannelID != "source-default" {
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
		info.Binding.ExternalChannelID != "source-default" {
		t.Fatalf("component info = %#v", info)
	}
}

func TestServiceRuntimeImageTargetsDiscoversProviders(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})
	saveComponent(t, storage, "image", "runner")
	saveComponent(t, storage, "plain", "ignored")

	targets, err := svc.RuntimeImageTargets(ctx)
	if err != nil {
		t.Fatalf("RuntimeImageTargets() error = %v", err)
	}
	if got, want := len(targets), 1; got != want {
		t.Fatalf("targets = %d, want %d: %#v", got, want, targets)
	}
	target := targets[0]
	if target.Ref != "image/runner" || target.Image != "ctgbot-fake:latest" || target.Dockerfile != "fake.Dockerfile" {
		t.Fatalf("target = %#v", target)
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

func saveChat(t *testing.T, storage repository.Storage, label string) *coremodel.Chat {
	t.Helper()
	chat := &coremodel.Chat{Label: label, Enabled: true}
	if err := storage.Chats().Save(context.Background(), chat); err != nil {
		t.Fatal(err)
	}
	return chat
}

func saveChatComponent(t *testing.T, storage repository.Storage, chatID modeluuid.UUID, componentID modeluuid.UUID, role coremodel.ChatComponentRole, externalChannelID string) coremodel.ChatComponent {
	t.Helper()
	binding := coremodel.ChatComponent{ChatID: chatID, ComponentID: componentID, Role: role, ExternalChannelID: externalChannelID, Enabled: true}
	if err := storage.ChatComponents().Save(context.Background(), &binding); err != nil {
		t.Fatal(err)
	}
	return binding
}

func TestServiceAdmitInboundReturnsChannelAndFilters(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})

	chat := saveChat(t, storage, "Team")
	source := saveComponent(t, storage, "source", "inbox")
	filter := saveComponent(t, storage, "filters", "allowlist")
	sourceBinding := saveChatComponent(t, storage, chat.ID, source.ID, coremodel.ChatComponentRoleSource, "inbox")
	saveChatComponent(t, storage, chat.ID, source.ID, coremodel.ChatComponentRoleRelay, "inbox")
	if err := storage.InboundFilterBindings().Save(ctx, &coremodel.InboundFilterBinding{SourceBindingID: sourceBinding.ID, FilterComponentID: filter.ID, Enabled: true}); err != nil {
		t.Fatal(err)
	}

	admission, err := svc.AdmitInbound(ctx, component.InboundEvent{
		ComponentID: source.ID,
		Payload:     messagePayload("inbox", "hello"),
	})
	if err != nil {
		t.Fatalf("AdmitInbound() error = %v", err)
	}
	if admission.Rejected != nil {
		t.Fatalf("AdmitInbound() rejected = %#v", admission.Rejected)
	}
	if admission.Channel.Chat.ID != chat.ID || admission.Channel.SourceBinding.ID != sourceBinding.ID {
		t.Fatalf("channel = %#v", admission.Channel)
	}
	if got, want := len(admission.Filters), 1; got != want {
		t.Fatalf("filters = %d, want %d", got, want)
	}
}

func TestServiceAdmitInboundRejectsWithoutRelay(t *testing.T) {
	ctx := context.Background()
	storage := repository.NewMemory()
	svc := app.NewService(storage, fakeResolver{storage: storage})

	chat := saveChat(t, storage, "Team")
	source := saveComponent(t, storage, "source", "inbox")
	saveChatComponent(t, storage, chat.ID, source.ID, coremodel.ChatComponentRoleSource, "inbox")

	admission, err := svc.AdmitInbound(ctx, component.InboundEvent{
		ComponentID: source.ID,
		Payload:     messagePayload("inbox", "hello"),
	})
	if err != nil {
		t.Fatalf("AdmitInbound() error = %v", err)
	}
	if admission.Rejected == nil || admission.Rejected.Reason != "no-relay-binding" {
		t.Fatalf("rejection = %#v, want no-relay-binding", admission.Rejected)
	}
}

func messagePayload(channelID string, text string) message.InboundPayload {
	return message.InboundPayload{
		ProviderType:      "test",
		ProviderChannelID: channelID,
		Text:              message.TextMessage{Text: text},
	}
}
