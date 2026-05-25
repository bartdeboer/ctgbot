package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
)

const RegistryFilename = "models.json"

type Registry struct {
	DefaultModel  string                 `json:"default_model,omitempty"`
	DefaultModels map[string]string      `json:"default_models,omitempty"`
	Models        map[string]ModelRecord `json:"models,omitempty"`
}

type ModelRecord struct {
	URL         string                          `json:"url,omitempty"`
	Filename    string                          `json:"filename,omitempty"`
	Path        string                          `json:"path,omitempty"`
	Mode        string                          `json:"mode,omitempty"`
	Card        string                          `json:"card,omitempty"`
	SHA256      string                          `json:"sha256,omitempty"`
	MMProjPath  string                          `json:"mmproj_path,omitempty"`
	HostPort    int                             `json:"host_port,omitempty"`
	ContextSize int                             `json:"ctx_size,omitempty"`
	UBatchSize  int                             `json:"ubatch_size,omitempty"`
	GPULayers   int                             `json:"gpu_layers,omitempty"`
	MaxTokens   int                             `json:"max_tokens,omitempty"`
	Temperature float64                         `json:"temperature,omitempty"`
	Pooling     string                          `json:"pooling,omitempty"`
	Normalize   *bool                           `json:"normalize,omitempty"`
	Toolloop    component.ModelToolloopProfile  `json:"toolloop,omitempty"`
	ConfigKeys  map[string]ModelConfigKeyRecord `json:"config_keys,omitempty"`
}

type ModelConfigKeyRecord struct {
	Type    string   `json:"type,omitempty"`
	Help    string   `json:"help,omitempty"`
	Default string   `json:"default,omitempty"`
	Options []string `json:"options,omitempty"`
}

func loadRegistry(homePath string) (Registry, error) {
	path := filepath.Join(strings.TrimSpace(homePath), RegistryFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Registry{Models: map[string]ModelRecord{}}, nil
		}
		return Registry{}, fmt.Errorf("read model registry %s: %w", path, err)
	}
	var registry Registry
	if err := json.Unmarshal(data, &registry); err != nil {
		return Registry{}, fmt.Errorf("read model registry %s: %w", path, err)
	}
	registry.DefaultModel = cleanModelName(registry.DefaultModel)
	registry.DefaultModels = cleanDefaultModels(registry.DefaultModels)
	registry.Models = cleanRegistry(registry.Models)
	return registry, nil
}

func saveRegistry(homePath string, registry Registry) error {
	registry.DefaultModel = cleanModelName(registry.DefaultModel)
	registry.DefaultModels = cleanDefaultModels(registry.DefaultModels)
	registry.Models = cleanRegistry(registry.Models)
	data, err := json.MarshalIndent(registry, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return atomicWriteFile(filepath.Join(strings.TrimSpace(homePath), RegistryFilename), data, 0o644)
}

func cleanDefaultModels(defaults map[string]string) map[string]string {
	if len(defaults) == 0 {
		return nil
	}
	out := make(map[string]string, len(defaults))
	for mode, name := range defaults {
		mode = cleanModelMode(mode)
		name = cleanModelName(name)
		if mode == "" || name == "" {
			continue
		}
		out[mode] = name
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cleanRegistry(models map[string]ModelRecord) map[string]ModelRecord {
	if len(models) == 0 {
		return map[string]ModelRecord{}
	}
	out := make(map[string]ModelRecord, len(models))
	for name, record := range models {
		name = cleanModelName(name)
		if name == "" {
			continue
		}
		out[name] = cleanModelRecord(record)
	}
	return out
}

func cleanModelRecord(record ModelRecord) ModelRecord {
	record.URL = strings.TrimSpace(record.URL)
	record.Filename = strings.TrimSpace(record.Filename)
	record.Path = strings.TrimSpace(record.Path)
	record.Mode = cleanModelMode(record.Mode)
	record.Card = strings.TrimSpace(record.Card)
	record.SHA256 = strings.TrimSpace(record.SHA256)
	record.MMProjPath = strings.TrimSpace(record.MMProjPath)
	record.Pooling = strings.TrimSpace(record.Pooling)
	record.Toolloop = cleanModelToolloopProfile(record.Toolloop)
	record.ConfigKeys = cleanModelConfigKeys(record.ConfigKeys)
	return record
}

func modelRecordFromComponent(model component.Model) ModelRecord {
	normalize := &model.Normalize
	if model.Mode != component.ModelModeEmbedding {
		normalize = nil
	}
	return ModelRecord{
		URL:         model.URL,
		Filename:    model.Filename,
		Path:        model.Path,
		Mode:        string(model.Mode),
		SHA256:      model.SHA256,
		MMProjPath:  model.MMProjPath,
		HostPort:    model.HostPort,
		ContextSize: model.ContextSize,
		UBatchSize:  model.UBatchSize,
		GPULayers:   model.GPULayers,
		MaxTokens:   model.MaxTokens,
		Temperature: model.Temperature,
		Pooling:     model.Pooling,
		Normalize:   normalize,
	}
}

func cleanModelToolloopProfile(profile component.ModelToolloopProfile) component.ModelToolloopProfile {
	profile.PromptInstructions = strings.TrimSpace(profile.PromptInstructions)
	profile.ToolInstructions = strings.TrimSpace(profile.ToolInstructions)
	profile.ReasoningFormat = strings.TrimSpace(profile.ReasoningFormat)
	profile.ToolCallFormat = strings.TrimSpace(profile.ToolCallFormat)
	return profile
}

func cleanModelConfigKeys(keys map[string]ModelConfigKeyRecord) map[string]ModelConfigKeyRecord {
	if len(keys) == 0 {
		return nil
	}
	out := make(map[string]ModelConfigKeyRecord, len(keys))
	for key, record := range keys {
		key = configsurface.NormalizeKey(key)
		if key == "" {
			continue
		}
		out[key] = cleanModelConfigKeyRecord(record)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cleanModelConfigKeyRecord(record ModelConfigKeyRecord) ModelConfigKeyRecord {
	record.Type = strings.TrimSpace(strings.ToLower(record.Type))
	record.Help = strings.TrimSpace(record.Help)
	record.Default = strings.TrimSpace(record.Default)
	record.Options = cleanStringList(record.Options)
	return record
}

func cleanStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
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
		return string(component.ModelModeEmbedding)
	case "asr", "transcription", "transcribe", "speech-to-text", "stt":
		return string(component.ModelModeASR)
	case "tts", "speech", "synthesis", "speech-synthesis", "text-to-speech":
		return string(component.ModelModeTTS)
	default:
		return string(component.ModelModeCompletion)
	}
}

func modelNormalize(model ModelRecord) bool {
	if model.Normalize != nil {
		return *model.Normalize
	}
	return cleanModelMode(model.Mode) == string(component.ModelModeEmbedding)
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

func downloadFile(url string, target string, wantSHA256 string) error {
	resp, err := http.Get(strings.TrimSpace(url))
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
