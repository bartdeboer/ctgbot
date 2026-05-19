package configsurface

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type fakeSurface struct {
	values map[string]string
}

func (s *fakeSurface) ConfigSchema(ctx context.Context, req commandengine.Request) (ConfigSchema, error) {
	_, _ = ctx, req
	return ConfigSchema{Fields: []FieldSchema{
		{Key: "model", Help: "Model", Type: FieldTypeString, Writable: true, Default: "default-model", Options: []string{"a", "b"}},
		{Key: "token", Help: "Token", Type: FieldTypeString, Writable: false, Secret: true, Default: "secret"},
	}}, nil
}

func (s *fakeSurface) ConfigGet(ctx context.Context, req commandengine.Request, key string) (string, error) {
	_, _ = ctx, req
	return s.values[NormalizeKey(key)], nil
}

func (s *fakeSurface) ConfigSet(ctx context.Context, req commandengine.Request, key string, value string) error {
	_, _ = ctx, req
	s.values[NormalizeKey(key)] = strings.TrimSpace(value)
	return nil
}

func (s *fakeSurface) ConfigUnset(ctx context.Context, req commandengine.Request, key string) error {
	_, _ = ctx, req
	delete(s.values, NormalizeKey(key))
	return nil
}

func TestConfigSurfaceCommandAdapter(t *testing.T) {
	surface := &fakeSurface{values: map[string]string{"model": "a", "token": "secret-value"}}
	registry := commandengine.NewRegistry()
	if err := RegisterCommandHandlers(registry, surface); err != nil {
		t.Fatalf("RegisterCommandHandlers() error = %v", err)
	}
	router, err := commandengine.NewRouter(CommandDefinitions(DefinitionOptions{
		Sources:       []commandengine.Source{commandengine.SourceMessage},
		Policy:        simplerbac.Public(),
		SupportsUnset: true,
	}), commandengine.SourceMessage)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	engine := commandengine.NewEngine(router, registry)
	base := commandengine.Request{Context: commandengine.Context{Source: commandengine.SourceMessage}}

	list, err := engine.Run(context.Background(), base, []string{"config", "list"})
	if err != nil {
		t.Fatalf("config list error = %v", err)
	}
	for _, want := range []string{"model=a - Model", "default: default-model", "options: a, b", "token=[redacted]", "read-only"} {
		if !strings.Contains(list.Text, want) {
			t.Fatalf("config list missing %q:\n%s", want, list.Text)
		}
	}

	set, err := engine.Run(context.Background(), base, []string{"config", "set", "model", "b"})
	if err != nil {
		t.Fatalf("config set error = %v", err)
	}
	if got, want := set.Text, "model=b"; got != want {
		t.Fatalf("set result = %q, want %q", got, want)
	}

	get, err := engine.Run(context.Background(), base, []string{"config", "get", "model"})
	if err != nil {
		t.Fatalf("config get error = %v", err)
	}
	for _, want := range []string{"model=b", "type: string", "default: default-model", "options: a, b", "writable: true"} {
		if !strings.Contains(get.Text, want) {
			t.Fatalf("config get missing %q:\n%s", want, get.Text)
		}
	}

	token, err := engine.Run(context.Background(), base, []string{"config", "get", "token"})
	if err != nil {
		t.Fatalf("config get token error = %v", err)
	}
	if strings.Contains(token.Text, "secret-value") || !strings.Contains(token.Text, "token=[redacted]") {
		t.Fatalf("secret config get = %q, want redacted", token.Text)
	}

	if _, err := engine.Run(context.Background(), base, []string{"config", "unset", "token"}); err == nil || !strings.Contains(err.Error(), "read-only") {
		t.Fatalf("config unset read-only error = %v, want read-only", err)
	}
}
