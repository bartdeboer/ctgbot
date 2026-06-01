package ops

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/app"
	"github.com/bartdeboer/ctgbot/internal/appstate"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clistate"
)

func TestOpsComponentsAddDefaultsToCurrentChatAndCommandRole(t *testing.T) {
	chatID := modeluuid.New()
	service := &fakeService{}
	engine := newTestEngine(t, New(service))

	result, err := engine.Run(context.Background(), baseRequest(chatID), []string{"ops", "components", "add", "search"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := service.addChatID, chatID; got != want {
		t.Fatalf("add chat id = %s, want %s", got, want)
	}
	if got, want := service.addRole, coremodel.ChatComponentRoleCommand; got != want {
		t.Fatalf("add role = %s, want %s", got, want)
	}
	if got, want := service.addComponent, "search"; got != want {
		t.Fatalf("add component = %q, want %q", got, want)
	}
	if !strings.Contains(result.Text, "ops component added") {
		t.Fatalf("result text = %q, want add confirmation", result.Text)
	}
}

func TestOpsComponentsListCanUseExplicitChat(t *testing.T) {
	chatID := modeluuid.New()
	service := &fakeService{resolvedChatID: chatID, list: []app.ChatComponentInfo{{
		Binding:      coremodel.ChatComponent{Role: coremodel.ChatComponentRoleCommand},
		ComponentRef: "search",
		Runtime:      "local",
	}}}
	engine := newTestEngine(t, New(service))

	result, err := engine.Run(context.Background(), baseRequest(modeluuid.UUID{}), []string{"ops", "components", "list", "--chat", "main"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got, want := service.resolvedRef, "main"; got != want {
		t.Fatalf("resolved ref = %q, want %q", got, want)
	}
	if !strings.Contains(result.Text, "search\trole=command\truntime=local") {
		t.Fatalf("result text = %q, want search binding", result.Text)
	}
}

func TestOpsComponentsRemoveRequiresChatContext(t *testing.T) {
	engine := newTestEngine(t, New(&fakeService{}))
	_, err := engine.Run(context.Background(), baseRequest(modeluuid.UUID{}), []string{"ops", "components", "remove", "search"})
	if err == nil || !strings.Contains(err.Error(), "missing chat") {
		t.Fatalf("Run() error = %v, want missing chat", err)
	}
}

func TestOpsConfigSetGetUnsetLayer(t *testing.T) {
	cfg := newTestConfig(t)
	engine := newTestEngine(t, New(&fakeService{}, cfg))

	if _, err := engine.Run(context.Background(), baseRequest(modeluuid.New()), []string{"ops", "config", "set", "10-agent", "workspaces.agent.path", "workspaces/agent"}); err != nil {
		t.Fatalf("config set error = %v", err)
	}
	result, err := engine.Run(context.Background(), baseRequest(modeluuid.New()), []string{"ops", "config", "get", "workspaces.agent.path"})
	if err != nil {
		t.Fatalf("config get error = %v", err)
	}
	if !strings.Contains(result.Text, `"workspaces/agent"`) || !strings.Contains(result.Text, "10-agent.json") {
		t.Fatalf("config get text = %q, want value and layer source", result.Text)
	}
	if _, err := os.Stat(filepath.Join(cfg.RootDir(), "config.d", "10-agent.json")); err != nil {
		t.Fatalf("expected config.d layer file: %v", err)
	}

	if _, err := engine.Run(context.Background(), baseRequest(modeluuid.New()), []string{"ops", "config", "unset", "10-agent", "workspaces.agent.path"}); err != nil {
		t.Fatalf("config unset error = %v", err)
	}
	result, err = engine.Run(context.Background(), baseRequest(modeluuid.New()), []string{"ops", "config", "get", "workspaces.agent.path"})
	if err != nil {
		t.Fatalf("config get after unset error = %v", err)
	}
	if !strings.Contains(result.Text, "not set") {
		t.Fatalf("config get after unset text = %q, want not set", result.Text)
	}
}

func TestOpsConfigLayersListsJSONLayers(t *testing.T) {
	cfg := newTestConfig(t)
	engine := newTestEngine(t, New(&fakeService{}, cfg))
	if err := os.MkdirAll(filepath.Join(cfg.RootDir(), "config.d"), 0o755); err != nil {
		t.Fatalf("MkdirAll(config.d) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.RootDir(), "config.d", "10-agent.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatalf("WriteFile layer error = %v", err)
	}

	result, err := engine.Run(context.Background(), baseRequest(modeluuid.New()), []string{"ops", "config", "layers"})
	if err != nil {
		t.Fatalf("config layers error = %v", err)
	}
	if !strings.Contains(result.Text, "10-agent.json") {
		t.Fatalf("config layers text = %q, want layer name", result.Text)
	}
}

func newTestEngine(t *testing.T, surface *Component) *commandengine.Engine {
	t.Helper()
	engine, err := commandset.NewBoundEngineForSource(commandengine.SourceMessage, []commandset.BoundSurface{{
		Surface:       surface,
		ComponentRef:  "ops",
		ComponentType: Type,
	}})
	if err != nil {
		t.Fatalf("NewBoundEngineForSource() error = %v", err)
	}
	return engine
}

func baseRequest(chatID modeluuid.UUID) commandengine.Request {
	return commandengine.Request{Context: commandengine.Context{ChatID: chatID, Actor: coremodel.Actor{ID: "agent", Roles: []simplerbac.Role{simplerbac.RoleAgent}}}}
}

func newTestConfig(t *testing.T) *appstate.Config {
	t.Helper()
	root := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewCwd() error = %v", err)
	}
	cfg, err := appstate.NewConfig(filepath.Join(root, ".ctgbot"), store)
	if err != nil {
		t.Fatalf("NewConfig() error = %v", err)
	}
	if err := cfg.EnsurePaths(); err != nil {
		t.Fatalf("EnsurePaths() error = %v", err)
	}
	return cfg
}

type fakeService struct {
	resolvedRef    string
	resolvedChatID modeluuid.UUID
	addChatID      modeluuid.UUID
	addRole        coremodel.ChatComponentRole
	addComponent   string
	listChatID     modeluuid.UUID
	list           []app.ChatComponentInfo
}

func (f *fakeService) ResolveChatRef(ctx context.Context, ref string) (modeluuid.UUID, error) {
	_ = ctx
	f.resolvedRef = ref
	if f.resolvedChatID.IsNull() {
		f.resolvedChatID = modeluuid.New()
	}
	return f.resolvedChatID, nil
}

func (f *fakeService) AddChatComponent(ctx context.Context, chatID modeluuid.UUID, role coremodel.ChatComponentRole, componentRef string, externalChannelID string) (app.ChatComponentAddResult, error) {
	_, _ = ctx, externalChannelID
	f.addChatID = chatID
	f.addRole = role
	f.addComponent = componentRef
	return app.ChatComponentAddResult{Binding: coremodel.ChatComponent{ChatID: chatID, Role: role}, ComponentRef: componentRef, Runtime: "local"}, nil
}

func (f *fakeService) RemoveChatComponent(ctx context.Context, chatID modeluuid.UUID, role coremodel.ChatComponentRole, componentRef string) (app.ChatComponentRemoveResult, error) {
	_, _ = ctx, componentRef
	return app.ChatComponentRemoveResult{Binding: coremodel.ChatComponent{ChatID: chatID, Role: role}, ComponentRef: componentRef, Removed: true}, nil
}

func (f *fakeService) ListChatComponents(ctx context.Context, chatID modeluuid.UUID) ([]app.ChatComponentInfo, error) {
	_ = ctx
	f.listChatID = chatID
	return f.list, nil
}
