package ops

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/app"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/commandset"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
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
