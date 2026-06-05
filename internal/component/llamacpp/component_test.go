package llamacpp

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/component"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

func TestComponentManagedFiles(t *testing.T) {
	t.Parallel()

	component := &Component{}
	got := component.ManagedFiles()
	if len(got) != 2 {
		t.Fatalf("len(ManagedFiles) = %d, want 2", len(got))
	}
	if got[0].RelativePath != runtimepkg.ConfigFilename {
		t.Fatalf("ManagedFiles[0] = %q, want %q", got[0].RelativePath, runtimepkg.ConfigFilename)
	}
	if got[1].RelativePath != ComponentConfigFilename {
		t.Fatalf("ManagedFiles[1] = %q, want %q", got[1].RelativePath, ComponentConfigFilename)
	}
}

func TestServiceSpecUsesComponentConfig(t *testing.T) {
	t.Parallel()

	spec := serviceSpec(resolvedModel{
		Name:        "qwen",
		ModelPath:   "/srv/models/qwen/model.gguf",
		MMProjPath:  "/srv/mmproj/mmproj.gguf",
		HostPort:    18080,
		ContextSize: 4096,
		GPULayers:   48,
	})
	if got, want := spec.BaseURL, "http://127.0.0.1:18080"; got != want {
		t.Fatalf("BaseURL = %q, want %q", got, want)
	}
	if !slices.Equal(spec.Ports, []string{"127.0.0.1:18080:8080"}) {
		t.Fatalf("Ports = %#v", spec.Ports)
	}
	if len(spec.Mounts) != 2 {
		t.Fatalf("len(Mounts) = %d, want 2: %#v", len(spec.Mounts), spec.Mounts)
	}
	if !slices.Contains(spec.Cmd, "--mmproj") {
		t.Fatalf("Cmd missing --mmproj: %#v", spec.Cmd)
	}
	if !slices.Contains(spec.Cmd, "/mmproj/mmproj.gguf") {
		t.Fatalf("Cmd missing /mmproj/mmproj.gguf: %#v", spec.Cmd)
	}
}

func TestServiceSpecMountsChatTemplateFileForCompletionModels(t *testing.T) {
	t.Parallel()

	spec := serviceSpec(resolvedModel{
		Name:             "qwen",
		ModelPath:        "/srv/models/qwen/model.gguf",
		ChatTemplatePath: "/srv/templates/qwen35-no-prefill-think.jinja",
		HostPort:         18080,
		ContextSize:      4096,
		GPULayers:        48,
	})

	wantArgs := []string{"--jinja", "--chat-template-file", "/templates/qwen35-no-prefill-think.jinja"}
	for _, arg := range wantArgs {
		if !slices.Contains(spec.Cmd, arg) {
			t.Fatalf("Cmd missing %s: %#v", arg, spec.Cmd)
		}
	}
	jinjaIndex := slices.Index(spec.Cmd, "--jinja")
	templateIndex := slices.Index(spec.Cmd, "--chat-template-file")
	if jinjaIndex < 0 || templateIndex < 0 || jinjaIndex > templateIndex {
		t.Fatalf("--jinja must precede --chat-template-file: %#v", spec.Cmd)
	}
	found := false
	for _, mount := range spec.Mounts {
		if mount.Source == "/srv/templates" && mount.Target == "/templates" && mount.ReadOnly {
			found = true
		}
	}
	if !found {
		t.Fatalf("template mount not found: %#v", spec.Mounts)
	}
}

func TestServiceSpecIgnoresChatTemplateForEmbeddingModels(t *testing.T) {
	t.Parallel()

	spec := serviceSpec(resolvedModel{
		Name:             "embed",
		ModelPath:        "/srv/models/embed/model.gguf",
		ChatTemplatePath: "/srv/templates/qwen.jinja",
		Mode:             "embedding",
		HostPort:         18081,
		ContextSize:      4096,
		GPULayers:        48,
	})

	if slices.Contains(spec.Cmd, "--chat-template-file") {
		t.Fatalf("embedding cmd should not include chat template: %#v", spec.Cmd)
	}
	for _, mount := range spec.Mounts {
		if mount.Target == "/templates" {
			t.Fatalf("embedding mounts should not include templates: %#v", spec.Mounts)
		}
	}
}

func TestApplyReasoningModeSetsLlamaCppThinkingKwarg(t *testing.T) {
	body := map[string]any{
		"chat_template_kwargs": map[string]any{
			"other": "value",
		},
	}

	applyReasoningMode(body, component.ReasoningDisabled)

	kwargs, ok := body["chat_template_kwargs"].(map[string]any)
	if !ok {
		t.Fatalf("chat_template_kwargs = %#v, want object", body["chat_template_kwargs"])
	}
	if got, want := kwargs["enable_thinking"], false; got != want {
		t.Fatalf("enable_thinking = %#v, want %#v", got, want)
	}
	if got, want := kwargs["other"], "value"; got != want {
		t.Fatalf("other = %#v, want %#v", got, want)
	}
}

func TestProviderOptionsAreClonedBeforeReasoningMerge(t *testing.T) {
	options := map[string]any{
		"chat_template_kwargs": map[string]any{
			"enable_thinking": true,
		},
	}

	body := cloneProviderOptions(options)
	applyReasoningMode(body, component.ReasoningDisabled)

	original := options["chat_template_kwargs"].(map[string]any)
	if got, want := original["enable_thinking"], true; got != want {
		t.Fatalf("original enable_thinking = %#v, want %#v", got, want)
	}
	data, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(data), `{"chat_template_kwargs":{"enable_thinking":false}}`; got != want {
		t.Fatalf("body json = %s, want %s", got, want)
	}
}

func TestCompletionRequestBodyAllowsExplicitZeroTemperature(t *testing.T) {
	zero := 0.0

	body := completionRequestBody(resolvedModel{Name: "qwen", Temperature: 0.7}, component.CompletionRequest{
		Prompt: component.CompletionPrompt{Messages: []component.CompletionMessage{{
			Role:    component.CompletionRoleUser,
			Content: "hello",
		}}},
		Temperature: &zero,
	})

	if got, ok := body["temperature"].(float64); !ok || got != 0 {
		t.Fatalf("temperature = %#v, want explicit 0", body["temperature"])
	}
}

func TestServiceSpecCanExposePortToSandboxes(t *testing.T) {
	t.Parallel()

	spec := serviceSpec(resolvedModel{
		Name:              "qwen",
		ModelPath:         "/srv/models/qwen/model.gguf",
		HostPort:          18080,
		ContextSize:       4096,
		GPULayers:         48,
		ExposeToSandboxes: true,
	})
	if !slices.Equal(spec.Ports, []string{"18080:8080"}) {
		t.Fatalf("Ports = %#v", spec.Ports)
	}
}
