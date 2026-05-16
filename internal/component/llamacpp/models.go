package llamacpp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const ModelsFilename = "models.json"

type ModelRegistry struct {
	Models map[string]ModelConfig `json:"models,omitempty"`
}

type ModelConfig struct {
	URL         string  `json:"url,omitempty"`
	Filename    string  `json:"filename,omitempty"`
	Path        string  `json:"path,omitempty"`
	SHA256      string  `json:"sha256,omitempty"`
	MMProjPath  string  `json:"mmproj_path,omitempty"`
	HostPort    int     `json:"host_port,omitempty"`
	ContextSize int     `json:"ctx_size,omitempty"`
	GPULayers   int     `json:"gpu_layers,omitempty"`
	MaxTokens   int     `json:"max_tokens,omitempty"`
	Temperature float64 `json:"temperature,omitempty"`
}

type resolvedModel struct {
	Name        string
	ModelPath   string
	MMProjPath  string
	HostPort    int
	ContextSize int
	GPULayers   int
	MaxTokens   int
	Temperature float64
}

func loadModelRegistry(homePath string) (ModelRegistry, error) {
	path := filepath.Join(strings.TrimSpace(homePath), ModelsFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ModelRegistry{Models: map[string]ModelConfig{}}, nil
		}
		return ModelRegistry{}, fmt.Errorf("read llama.cpp models registry %s: %w", path, err)
	}
	var registry ModelRegistry
	if err := json.Unmarshal(data, &registry); err != nil {
		return ModelRegistry{}, fmt.Errorf("read llama.cpp models registry %s: %w", path, err)
	}
	registry.Models = cleanModelRegistry(registry.Models)
	return registry, nil
}

func saveModelRegistry(homePath string, registry ModelRegistry) error {
	registry.Models = cleanModelRegistry(registry.Models)
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWriteFile(filepath.Join(strings.TrimSpace(homePath), ModelsFilename), data, 0o644)
}

func cleanModelRegistry(models map[string]ModelConfig) map[string]ModelConfig {
	if len(models) == 0 {
		return map[string]ModelConfig{}
	}
	out := make(map[string]ModelConfig, len(models))
	for name, model := range models {
		name = cleanModelName(name)
		if name == "" {
			continue
		}
		model.URL = strings.TrimSpace(model.URL)
		model.Filename = strings.TrimSpace(model.Filename)
		model.Path = strings.TrimSpace(model.Path)
		model.SHA256 = strings.TrimSpace(model.SHA256)
		model.MMProjPath = strings.TrimSpace(model.MMProjPath)
		out[name] = model
	}
	return out
}

func sortedModelNames(models map[string]ModelConfig) []string {
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
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

func (c *Component) resolveModel(name string) (resolvedModel, error) {
	if c == nil {
		return resolvedModel{}, fmt.Errorf("missing llama.cpp component")
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = strings.TrimSpace(c.componentConfig.DefaultModel)
	}
	if name == "" && strings.TrimSpace(c.componentConfig.ModelPath) != "" {
		return c.resolveLegacyModel(), nil
	}
	if name == "" && len(c.models.Models) == 1 {
		for only := range c.models.Models {
			name = only
		}
	}
	if name == "" {
		return resolvedModel{}, fmt.Errorf("missing llama.cpp model; configure default_model or pass a model name")
	}
	name = cleanModelName(name)
	model, ok := c.models.Models[name]
	if !ok {
		return resolvedModel{}, fmt.Errorf("llama.cpp model not installed: %s", name)
	}
	resolved := resolvedModel{
		Name:        name,
		ModelPath:   c.modelPath(name, model),
		MMProjPath:  firstNonEmpty(model.MMProjPath, c.componentConfig.MMProjPath),
		HostPort:    firstPositive(model.HostPort, c.componentConfig.HostPort),
		ContextSize: firstPositive(model.ContextSize, c.componentConfig.ContextSize),
		GPULayers:   firstPositive(model.GPULayers, c.componentConfig.GPULayers),
		MaxTokens:   firstPositive(model.MaxTokens, c.componentConfig.MaxTokens),
		Temperature: firstPositiveFloat(model.Temperature, c.componentConfig.Temperature),
	}
	if strings.TrimSpace(resolved.ModelPath) == "" {
		return resolvedModel{}, fmt.Errorf("llama.cpp model %s has no model file", name)
	}
	return resolved, nil
}

func (c *Component) resolveLegacyModel() resolvedModel {
	return resolvedModel{
		Name:        "default",
		ModelPath:   c.componentConfig.ModelPath,
		MMProjPath:  c.componentConfig.MMProjPath,
		HostPort:    c.componentConfig.HostPort,
		ContextSize: c.componentConfig.ContextSize,
		GPULayers:   c.componentConfig.GPULayers,
		MaxTokens:   c.componentConfig.MaxTokens,
		Temperature: c.componentConfig.Temperature,
	}
}

func (c *Component) modelPath(name string, model ModelConfig) string {
	if filepath.IsAbs(model.Path) {
		return model.Path
	}
	if strings.TrimSpace(model.Path) != "" {
		return filepath.Join(c.home.Path, model.Path)
	}
	filename := firstNonEmpty(model.Filename, filenameFromURL(model.URL))
	if filename == "" {
		return ""
	}
	return filepath.Join(c.home.Path, "models", name, filename)
}

func filenameFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	base := filepath.Base(raw)
	if base == "." || base == "/" {
		return ""
	}
	return base
}

func (c *Component) installModel(name string, model ModelConfig) (ModelConfig, error) {
	if c == nil {
		return ModelConfig{}, fmt.Errorf("missing llama.cpp component")
	}
	name = cleanModelName(name)
	if name == "" {
		return ModelConfig{}, fmt.Errorf("missing model name")
	}
	model.URL = strings.TrimSpace(model.URL)
	if model.URL == "" {
		return ModelConfig{}, fmt.Errorf("missing model url")
	}
	model.Filename = firstNonEmpty(model.Filename, filenameFromURL(model.URL))
	if model.Filename == "" {
		return ModelConfig{}, fmt.Errorf("missing model filename")
	}
	target := filepath.Join(c.home.Path, "models", name, model.Filename)
	if err := downloadFile(model.URL, target, model.SHA256); err != nil {
		return ModelConfig{}, err
	}
	model.Path = filepath.ToSlash(filepath.Join("models", name, model.Filename))
	if c.models.Models == nil {
		c.models.Models = map[string]ModelConfig{}
	}
	c.models.Models[name] = model
	if err := saveModelRegistry(c.home.Path, c.models); err != nil {
		return ModelConfig{}, err
	}
	return model, nil
}

func downloadFile(url string, target string, wantSHA256 string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("download %s: %s", url, resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	tmp := target + ".tmp"
	file, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	hash := sha256.New()
	_, copyErr := io.Copy(file, io.TeeReader(resp.Body, hash))
	closeErr := file.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return closeErr
	}
	if wantSHA256 = strings.TrimSpace(wantSHA256); wantSHA256 != "" {
		got := hex.EncodeToString(hash.Sum(nil))
		if !strings.EqualFold(got, wantSHA256) {
			_ = os.Remove(tmp)
			return fmt.Errorf("sha256 mismatch for %s: got %s want %s", target, got, wantSHA256)
		}
	}
	return os.Rename(tmp, target)
}

func atomicWriteFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
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
