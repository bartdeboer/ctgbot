package whispercpp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

const (
	Type                    = "whispercpp"
	ComponentConfigFilename = "component.json"
	DefaultImage            = "ghcr.io/ggml-org/whisper.cpp:main-cuda"
	DefaultWhisperCommand   = "/app/build/bin/whisper-cli"
	DefaultLDLibraryPath    = "/usr/local/cuda/lib64:/app/build/src:/app/build/ggml/src:/app/build/ggml/src/ggml-cuda"
)

type ComponentConfig struct {
	ModelStore     string   `json:"model_store,omitempty"`
	DefaultModel   string   `json:"default_model,omitempty"`
	Language       string   `json:"language,omitempty"`
	Threads        int      `json:"threads,omitempty"`
	MaxConcurrent  int      `json:"max_concurrent,omitempty"`
	FFMpegCommand  string   `json:"ffmpeg_command,omitempty"`
	WhisperCommand string   `json:"whisper_command,omitempty"`
	WhisperArgs    []string `json:"whisper_args,omitempty"`
}

func loadRuntimeConfig(homePath string) (runtimepkg.BindConfig, error) {
	config, err := runtimepkg.LoadBindConfig(homePath)
	if err != nil {
		return runtimepkg.BindConfig{}, err
	}
	config.Image = firstNonEmpty(config.Image, DefaultImage)
	config.IdleTimeout = firstNonEmpty(config.IdleTimeout, "30s")
	config.GPUs = firstNonEmpty(config.GPUs, "all")
	config.Env = runtimepkg.MergeEnv(
		[]string{"LD_LIBRARY_PATH=" + DefaultLDLibraryPath},
		config.Env,
	)
	if len(config.Cmd) == 0 {
		config.Cmd = []string{"tail", "-f", "/dev/null"}
	}
	return config.Clean(), nil
}

func loadComponentConfig(homePath string) (ComponentConfig, error) {
	var config ComponentConfig
	path := filepath.Join(strings.TrimSpace(homePath), ComponentConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return config.withDefaults(), nil
		}
		return ComponentConfig{}, fmt.Errorf("read whispercpp component config %s: %w", path, err)
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return config.withDefaults(), nil
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return ComponentConfig{}, fmt.Errorf("read whispercpp component config %s: %w", path, err)
	}
	return config.withDefaults(), nil
}

func (c ComponentConfig) withDefaults() ComponentConfig {
	c.ModelStore = firstNonEmpty(c.ModelStore, "model")
	c.DefaultModel = strings.TrimSpace(c.DefaultModel)
	c.Language = strings.TrimSpace(c.Language)
	if c.MaxConcurrent == 0 {
		c.MaxConcurrent = 1
	}
	c.FFMpegCommand = firstNonEmpty(c.FFMpegCommand, "ffmpeg")
	c.WhisperCommand = firstNonEmpty(c.WhisperCommand, DefaultWhisperCommand)
	c.WhisperArgs = cleanArgs(c.WhisperArgs)
	return c
}

func cleanArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		out = append(out, strings.TrimSpace(arg))
	}
	return out
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
