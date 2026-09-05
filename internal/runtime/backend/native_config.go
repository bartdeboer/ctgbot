package backend

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// NativeConfig belongs to a backend profile, not to a model or agent sandbox.
type NativeConfig struct {
	Executable string   `json:"executable"`
	Args       []string `json:"args,omitempty"`
}

func (c NativeConfig) validate() error {
	if !filepath.IsAbs(c.Executable) {
		return fmt.Errorf("native executable must be an absolute path")
	}
	return nil
}

func LoadNativeConfig(profile string) (*NativeConfig, error) {
	data, err := os.ReadFile(filepath.Join(profile, "runtime.json"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var config struct {
		Driver string        `json:"driver"`
		Native *NativeConfig `json:"native"`
	}
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}
	switch config.Driver {
	case "", "docker":
		if config.Native != nil {
			return nil, fmt.Errorf("native options require driver=native")
		}
		return nil, nil
	case "native":
		if config.Native == nil {
			return nil, fmt.Errorf("native driver requires native.executable")
		}
		return config.Native, config.Native.validate()
	default:
		return nil, fmt.Errorf("unknown backend driver %q", config.Driver)
	}
}
