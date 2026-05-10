package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const ConfigFilename = "runtime.json"

func LoadBindConfig(homePath string) (BindConfig, error) {
	path := filepath.Join(strings.TrimSpace(homePath), ConfigFilename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return BindConfig{}, nil
		}
		return BindConfig{}, fmt.Errorf("read runtime config %s: %w", path, err)
	}
	var config BindConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return BindConfig{}, fmt.Errorf("read runtime config %s: %w", path, err)
	}
	return config.Clean(), nil
}

func (c BindConfig) Clean() BindConfig {
	c.Image = strings.TrimSpace(c.Image)
	c.GPUs = strings.TrimSpace(c.GPUs)
	c.Seccomp = strings.ToLower(strings.TrimSpace(c.Seccomp))
	c.Env = cleanEnv(c.Env)
	return c
}

func (c BindConfig) WithEnv(extra ...string) BindConfig {
	c = c.Clean()
	c.Env = append(c.Env, cleanEnv(extra)...)
	return c
}

func (c BindConfig) WithEnvOverride(extra ...string) BindConfig {
	c = c.Clean()
	c.Env = MergeEnv(c.Env, extra)
	return c
}

func MergeEnv(base []string, extra []string) []string {
	out := make([]string, 0, len(base)+len(extra))
	for _, value := range cleanEnv(base) {
		if envKey(value) == "" {
			continue
		}
		out = append(out, value)
	}
	for _, value := range cleanEnv(extra) {
		key := envKey(value)
		if key == "" {
			continue
		}
		filtered := out[:0]
		for _, existing := range out {
			if envKey(existing) != key {
				filtered = append(filtered, existing)
			}
		}
		out = append(filtered, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func envKey(value string) string {
	key, _, ok := strings.Cut(strings.TrimSpace(value), "=")
	if !ok {
		return ""
	}
	return strings.TrimSpace(key)
}

func cleanEnv(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
