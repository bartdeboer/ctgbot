package llamacpp

import (
	"context"
	"fmt"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
)

type resolvedModel struct {
	Name              string
	ModelPath         string
	MMProjPath        string
	ChatTemplatePath  string
	Mode              string
	HostPort          int
	ContextSize       int
	UBatchSize        int
	GPULayers         int
	MaxTokens         int
	Temperature       float64
	Pooling           string
	Normalize         bool
	ExposeToSandboxes bool
}

func (c *Component) resolveModel(name string) (resolvedModel, error) {
	if c == nil {
		return resolvedModel{}, fmt.Errorf("missing llama.cpp component")
	}
	name = cleanModelName(name)
	if name == "" {
		name = cleanModelName(c.componentConfig.DefaultModel)
	}
	if name == "" && strings.TrimSpace(c.componentConfig.ModelPath) != "" {
		return c.resolveLegacyModel(), nil
	}
	registry, err := c.modelRegistry(context.Background())
	if err != nil {
		return resolvedModel{}, err
	}
	if name == "" {
		name, err = registry.DefaultModelForMode(context.Background(), component.ModelModeCompletion)
		if err != nil {
			return resolvedModel{}, err
		}
	}
	model, err := registry.GetModel(context.Background(), name)
	if err != nil {
		return resolvedModel{}, err
	}
	return c.resolveStoredModel(model), nil
}

func (c *Component) modelRegistry(ctx context.Context) (component.ModelRegistry, error) {
	if c == nil {
		return nil, fmt.Errorf("missing llama.cpp component")
	}
	if c.resolver == nil {
		return nil, fmt.Errorf("missing component resolver")
	}
	ref := strings.TrimSpace(c.componentConfig.ModelRegistry)
	if ref == "" {
		ref = "model"
	}
	registration, err := c.resolver.ResolveComponentRef(ctx, ref)
	if err != nil {
		return nil, err
	}
	loaded, err := c.resolver.ResolveComponent(ctx, registration.ID)
	if err != nil {
		return nil, err
	}
	if loaded == nil {
		return nil, fmt.Errorf("model registry not found: %s", ref)
	}
	registry, ok := loaded.Component.(component.ModelRegistry)
	if !ok {
		return nil, fmt.Errorf("component %s does not implement model registry", loaded.Registration.Ref())
	}
	return registry, nil
}

func (c *Component) resolveStoredModel(model component.Model) resolvedModel {
	return resolvedModel{
		Name:              cleanModelName(model.Name),
		ModelPath:         strings.TrimSpace(model.Path),
		MMProjPath:        firstNonEmpty(model.MMProjPath, c.componentConfig.MMProjPath),
		ChatTemplatePath:  strings.TrimSpace(model.ChatTemplatePath),
		Mode:              cleanModelMode(string(model.Mode)),
		HostPort:          firstPositive(model.HostPort, c.componentConfig.HostPort),
		ContextSize:       firstPositive(model.ContextSize, c.componentConfig.ContextSize),
		UBatchSize:        model.UBatchSize,
		GPULayers:         firstPositive(model.GPULayers, c.componentConfig.GPULayers),
		MaxTokens:         firstPositive(model.MaxTokens, c.componentConfig.MaxTokens),
		Temperature:       firstPositiveFloat(model.Temperature, c.componentConfig.Temperature),
		Pooling:           strings.TrimSpace(model.Pooling),
		Normalize:         model.Normalize,
		ExposeToSandboxes: c.componentConfig.ExposeToSandboxes,
	}
}

func (c *Component) resolveLegacyModel() resolvedModel {
	return resolvedModel{
		Name:              "default",
		ModelPath:         c.componentConfig.ModelPath,
		MMProjPath:        c.componentConfig.MMProjPath,
		Mode:              "completion",
		HostPort:          c.componentConfig.HostPort,
		ContextSize:       c.componentConfig.ContextSize,
		GPULayers:         c.componentConfig.GPULayers,
		MaxTokens:         c.componentConfig.MaxTokens,
		Temperature:       c.componentConfig.Temperature,
		ExposeToSandboxes: c.componentConfig.ExposeToSandboxes,
	}
}

func cleanModelName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		}
	}
	return strings.Trim(b.String(), "-_.")
}

func cleanModelMode(mode string) string {
	mode = strings.ToLower(strings.TrimSpace(mode))
	switch mode {
	case "embedding", "embed":
		return "embedding"
	case "asr", "transcription", "transcribe", "speech-to-text", "stt":
		return "asr"
	default:
		return "completion"
	}
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func firstPositiveFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
