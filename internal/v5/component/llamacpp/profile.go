package llamacpp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	Type         = "llamacpp"
	DefaultImage = "ghcr.io/ggml-org/llama.cpp:server-cuda"
)

type Profile struct {
	Image          string  `json:"image"`
	ModelPath      string  `json:"model_path"`
	MMProjPath     string  `json:"mmproj_path,omitempty"`
	HostPort       int     `json:"host_port"`
	ContextSize    int     `json:"ctx_size"`
	GPULayers      int     `json:"gpu_layers"`
	MaxTokens      int     `json:"max_tokens"`
	Temperature    float64 `json:"temperature"`
	StripReasoning bool    `json:"strip_reasoning"`
}

func loadProfile(homePath string, name string) (Profile, error) {
	profile := defaultProfile(name)
	path := filepath.Join(strings.TrimSpace(homePath), "config.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return profile.withDefaults(), nil
		}
		return Profile{}, err
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		return Profile{}, fmt.Errorf("read llamacpp config %s: %w", path, err)
	}
	return profile.withDefaults(), nil
}

func defaultProfile(name string) Profile {
	switch strings.TrimSpace(name) {
	case "qwen3-q5":
		return Profile{
			ModelPath: "/workspace/src/llm/models/qwen3-8b/Qwen3-8B-Q5_K_M.gguf",
			HostPort:  19081,
		}
	case "gemma4-e4b":
		return Profile{
			ModelPath:   "/workspace/src/llm/models/gemma4-e4b-gguf/gemma-4-E4B-it-Q4_K_M.gguf",
			MMProjPath:  "/workspace/src/llm/models/gemma4-e4b-gguf/mmproj-gemma-4-E4B-it-Q8_0.gguf",
			HostPort:    19082,
			ContextSize: 4096,
		}
	default:
		return Profile{
			ModelPath: "/workspace/src/llm/models/qwen3-8b/Qwen3-8B-Q4_K_M.gguf",
			HostPort:  19080,
		}
	}
}

func (p Profile) withDefaults() Profile {
	if strings.TrimSpace(p.Image) == "" {
		p.Image = DefaultImage
	}
	if p.HostPort == 0 {
		p.HostPort = 19080
	}
	if p.ContextSize == 0 {
		p.ContextSize = 8192
	}
	if p.GPULayers == 0 {
		p.GPULayers = 99
	}
	if p.MaxTokens == 0 {
		p.MaxTokens = 1024
	}
	if p.Temperature == 0 {
		p.Temperature = 0.2
	}
	p.StripReasoning = true
	return p
}
