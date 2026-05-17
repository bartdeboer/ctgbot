package model

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/go-clir"
)

func TestRegisterModelStoresAndResolvesModel(t *testing.T) {
	dir := t.TempDir()
	modelPath := filepath.Join(dir, "qwen.gguf")
	if err := os.WriteFile(modelPath, []byte("model"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	created, err := New(context.Background(), coremodel.Component{Type: Type, Name: Type}, nil, runtimepkg.Home{Path: dir}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	store := created.(*Component)
	model, err := store.RegisterModel(context.Background(), component.ModelInstallRequest{
		Model: component.Model{
			Name:        "Qwen Embed",
			Path:        modelPath,
			Mode:        component.ModelModeEmbedding,
			HostPort:    19100,
			ContextSize: 32768,
			Pooling:     "last",
			Normalize:   true,
		},
		Default: true,
	})
	if err != nil {
		t.Fatalf("RegisterModel() error = %v", err)
	}
	if model.Name != "qwenembed" {
		t.Fatalf("Name = %q, want cleaned qwenembed", model.Name)
	}
	if model.Path != modelPath || model.Mode != component.ModelModeEmbedding || !model.Normalize {
		t.Fatalf("model = %#v", model)
	}
	loaded, err := New(context.Background(), coremodel.Component{Type: Type, Name: Type}, nil, runtimepkg.Home{Path: dir}, nil)
	if err != nil {
		t.Fatalf("New() reload error = %v", err)
	}
	got, err := loaded.(*Component).GetModel(context.Background(), "")
	if err != nil {
		t.Fatalf("GetModel(default) error = %v", err)
	}
	if got.Name != model.Name || got.Path != modelPath {
		t.Fatalf("default model = %#v, want %#v", got, model)
	}
}

func TestComponentConfigModelPathResolvesRelativeRegistryPaths(t *testing.T) {
	home := t.TempDir()
	modelRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, ComponentConfigFilename), []byte(`{"model_path":`+quoteJSON(modelRoot)+`}`), 0o644); err != nil {
		t.Fatalf("WriteFile(component.json) error = %v", err)
	}
	modelPath := filepath.Join(modelRoot, "qwen", "model.gguf")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(modelPath, []byte("model"), 0o644); err != nil {
		t.Fatalf("WriteFile(model) error = %v", err)
	}
	created, err := New(context.Background(), coremodel.Component{Type: Type, Name: Type}, nil, runtimepkg.Home{Path: home}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	store := created.(*Component)
	model, err := store.RegisterModel(context.Background(), component.ModelInstallRequest{
		Model: component.Model{Name: "qwen", Path: "qwen/model.gguf"},
	})
	if err != nil {
		t.Fatalf("RegisterModel() error = %v", err)
	}
	if got, want := model.Path, modelPath; got != want {
		t.Fatalf("Path = %q, want %q", got, want)
	}
}

func TestLegacyInstalledModelPathStillResolvesAgainstProfileHome(t *testing.T) {
	home := t.TempDir()
	modelPath := filepath.Join(home, "models", "qwen", "model.gguf")
	if err := os.MkdirAll(filepath.Dir(modelPath), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(modelPath, []byte("model"), 0o644); err != nil {
		t.Fatalf("WriteFile(model) error = %v", err)
	}
	registry := Registry{Models: map[string]ModelRecord{
		"qwen": {Path: "models/qwen/model.gguf"},
	}}
	if err := saveRegistry(home, registry); err != nil {
		t.Fatalf("saveRegistry() error = %v", err)
	}
	created, err := New(context.Background(), coremodel.Component{Type: Type, Name: Type}, nil, runtimepkg.Home{Path: home}, nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	model, err := created.(*Component).GetModel(context.Background(), "qwen")
	if err != nil {
		t.Fatalf("GetModel() error = %v", err)
	}
	if got, want := model.Path, modelPath; got != want {
		t.Fatalf("Path = %q, want legacy %q", got, want)
	}
}

func TestModelCommandDefinitions(t *testing.T) {
	definitions := (&Component{}).CommandDefinitions()
	want := map[string]bool{
		"list":                   true,
		"install <name> <url>":   true,
		"register <name> <path>": true,
	}
	if len(definitions) != len(want) {
		t.Fatalf("len(CommandDefinitions)=%d want %d", len(definitions), len(want))
	}
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			t.Fatalf("definition %s invalid: %v", definition.CanonicalPattern(), err)
		}
		if !want[definition.CanonicalPattern()] {
			t.Fatalf("unexpected pattern %q", definition.CanonicalPattern())
		}
	}
}

func TestBuildRegisterCommandParsesEmbeddingFlags(t *testing.T) {
	built, err := buildRegisterCommand(testRequest(map[string]string{"name": "qwen", "path": "/models/qwen.gguf"}, []string{"--embedding", "--pooling", "last", "--host-port", "19100", "--default"}))
	if err != nil {
		t.Fatalf("buildRegisterCommand() error = %v", err)
	}
	cmd := built.(installCommand)
	if cmd.Name != "qwen" || cmd.Mode != component.ModelModeEmbedding || cmd.Pooling != "last" || cmd.HostPort != 19100 || !cmd.Default {
		t.Fatalf("cmd = %#v", cmd)
	}
}

func testRequest(params map[string]string, extra []string) *clir.Request {
	return &clir.Request{Params: params, Extra: extra}
}

func quoteJSON(value string) string {
	data, _ := json.Marshal(value)
	return string(data)
}
