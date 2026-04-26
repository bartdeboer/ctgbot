package appstate

import "strings"

type GlobalConfig struct {
	cfg *Config
}

func (c *Config) Global() GlobalConfig {
	return GlobalConfig{cfg: c}
}

func (g GlobalConfig) ProjectDir() string {
	store := g.store()
	if store == nil {
		return ""
	}
	return store.GetProjectDir()
}

func (g GlobalConfig) BuildCompilerPath() string {
	store := g.store()
	if store == nil {
		return ""
	}
	return strings.TrimSpace(store.GetString("build.compiler_path", ""))
}

func (g GlobalConfig) SetBuildCompilerPath(path string) error {
	store := g.store()
	if store == nil {
		return errMissingConfigStore()
	}
	return store.PersistString("build.compiler_path", strings.TrimSpace(path))
}

func (g GlobalConfig) store() interface {
	GetProjectDir() string
	GetString(string, string, ...*string) string
	PersistString(string, string) error
} {
	if g.cfg == nil {
		return nil
	}
	if g.cfg.global != nil {
		return g.cfg.global
	}
	return g.cfg.store
}
