package model

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

const Type = "model"

type Component struct {
	registration coremodel.Component
	home         runtimepkg.Home
	config       ComponentConfig
	registry     Registry
}

var _ component.Component = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)
var _ component.ModelRegistry = (*Component)(nil)

func New(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage) (component.Component, error) {
	_, _, _ = ctx, runtime, storage
	config, err := loadComponentConfig(home.Path)
	if err != nil {
		return nil, err
	}
	registry, err := loadRegistry(home.Path)
	if err != nil {
		return nil, err
	}
	return &Component{registration: registration, home: home, config: config, registry: registry}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: ComponentConfigFilename, Required: false, Sensitive: false},
		{RelativePath: RegistryFilename, Required: false, Sensitive: false},
	}
}

func (c *Component) ListModels(ctx context.Context) ([]component.Model, error) {
	_ = ctx
	if c == nil {
		return nil, fmt.Errorf("missing model component")
	}
	names := sortedModelNames(c.registry.Models)
	out := make([]component.Model, 0, len(names))
	for _, name := range names {
		out = append(out, c.resolve(name, c.registry.Models[name]))
	}
	return out, nil
}

func (c *Component) GetModel(ctx context.Context, name string) (component.Model, error) {
	_ = ctx
	if c == nil {
		return component.Model{}, fmt.Errorf("missing model component")
	}
	name = cleanModelName(name)
	if name == "" {
		name = strings.TrimSpace(c.registry.DefaultModel)
	}
	if name == "" && len(c.registry.Models) == 1 {
		for only := range c.registry.Models {
			name = only
		}
	}
	if name == "" {
		return component.Model{}, fmt.Errorf("missing model name")
	}
	model, ok := c.registry.Models[name]
	if !ok {
		return component.Model{}, fmt.Errorf("model not found: %s", name)
	}
	return c.resolve(name, model), nil
}

func (c *Component) DefaultModel(ctx context.Context) (string, error) {
	_ = ctx
	if c == nil {
		return "", fmt.Errorf("missing model component")
	}
	return strings.TrimSpace(c.registry.DefaultModel), nil
}

func (c *Component) DefaultModelForMode(ctx context.Context, mode component.ModelMode) (string, error) {
	_ = ctx
	if c == nil {
		return "", fmt.Errorf("missing model component")
	}
	return c.defaultModelForMode(mode), nil
}

func (c *Component) ModelCard(ctx context.Context, name string) (string, error) {
	_ = ctx
	if c == nil {
		return "", fmt.Errorf("missing model component")
	}
	_, record, err := c.lookupRecord(name)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(record.Card), nil
}

func (c *Component) SetModelCard(ctx context.Context, name string, text string) error {
	_ = ctx
	if c == nil {
		return fmt.Errorf("missing model component")
	}
	name, record, err := c.lookupRecord(name)
	if err != nil {
		return err
	}
	record.Card = strings.TrimSpace(text)
	c.registry.Models[name] = cleanModelRecord(record)
	return saveRegistry(c.home.Path, c.registry)
}

func (c *Component) ModelConfigSchema(ctx context.Context, name string) (configsurface.ConfigSchema, error) {
	_ = ctx
	if c == nil {
		return configsurface.ConfigSchema{}, fmt.Errorf("missing model component")
	}
	_, record, err := c.lookupRecord(name)
	if err != nil {
		return configsurface.ConfigSchema{}, err
	}
	keys := make([]string, 0, len(record.ConfigKeys))
	for key := range record.ConfigKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	fields := make([]configsurface.FieldSchema, 0, len(keys))
	for _, key := range keys {
		field := fieldSchemaFromRecord(key, record.ConfigKeys[key])
		fields = append(fields, field)
	}
	return configsurface.ConfigSchema{Fields: fields}, nil
}

func (c *Component) InstallModel(ctx context.Context, req component.ModelInstallRequest) (component.Model, error) {
	_ = ctx
	if c == nil {
		return component.Model{}, fmt.Errorf("missing model component")
	}
	name := cleanModelName(req.Name)
	if name == "" {
		return component.Model{}, fmt.Errorf("missing model name")
	}
	record := modelRecordFromComponent(req.Model)
	record.URL = strings.TrimSpace(record.URL)
	if record.URL == "" {
		return component.Model{}, fmt.Errorf("missing model url")
	}
	record.Filename = firstNonEmpty(record.Filename, filenameFromURL(record.URL))
	if record.Filename == "" {
		return component.Model{}, fmt.Errorf("missing model filename")
	}
	target := filepath.Join(c.config.ModelPath, name, record.Filename)
	if err := downloadFile(record.URL, target, record.SHA256); err != nil {
		return component.Model{}, err
	}
	record.Path = filepath.ToSlash(filepath.Join(name, record.Filename))
	return c.saveModel(name, record, req.Default)
}

func (c *Component) RegisterModel(ctx context.Context, req component.ModelInstallRequest) (component.Model, error) {
	_ = ctx
	if c == nil {
		return component.Model{}, fmt.Errorf("missing model component")
	}
	name := cleanModelName(req.Name)
	if name == "" {
		return component.Model{}, fmt.Errorf("missing model name")
	}
	record := modelRecordFromComponent(req.Model)
	if strings.TrimSpace(record.Path) == "" {
		return component.Model{}, fmt.Errorf("missing model path")
	}
	return c.saveModel(name, record, req.Default)
}

func (c *Component) saveModel(name string, record ModelRecord, makeDefault bool) (component.Model, error) {
	if c.registry.Models == nil {
		c.registry.Models = map[string]ModelRecord{}
	}
	c.registry.Models[name] = cleanModelRecord(record)
	if makeDefault {
		mode := cleanModelMode(record.Mode)
		if c.registry.DefaultModels == nil {
			c.registry.DefaultModels = map[string]string{}
		}
		c.registry.DefaultModels[mode] = name
		if mode == string(component.ModelModeCompletion) {
			c.registry.DefaultModel = name
		}
	}
	if err := saveRegistry(c.home.Path, c.registry); err != nil {
		return component.Model{}, err
	}
	return c.resolve(name, c.registry.Models[name]), nil
}

func (c *Component) lookupRecord(name string) (string, ModelRecord, error) {
	name = cleanModelName(name)
	if name == "" {
		name = strings.TrimSpace(c.registry.DefaultModel)
	}
	if name == "" && len(c.registry.Models) == 1 {
		for only := range c.registry.Models {
			name = only
		}
	}
	if name == "" {
		return "", ModelRecord{}, fmt.Errorf("missing model name")
	}
	record, ok := c.registry.Models[name]
	if !ok {
		return "", ModelRecord{}, fmt.Errorf("model not found: %s", name)
	}
	return name, record, nil
}

func fieldSchemaFromRecord(key string, record ModelConfigKeyRecord) configsurface.FieldSchema {
	fieldType := configsurface.FieldType(strings.TrimSpace(record.Type))
	if fieldType == "" && len(record.Options) > 0 {
		fieldType = configsurface.FieldTypeEnum
	}
	return configsurface.FieldSchema{
		Key:      configsurface.NormalizeKey(key),
		Help:     strings.TrimSpace(record.Help),
		Type:     fieldType,
		Writable: true,
		Default:  strings.TrimSpace(record.Default),
		Options:  append([]string(nil), record.Options...),
	}
}

func (c *Component) defaultModelForMode(mode component.ModelMode) string {
	modeKey := cleanModelMode(string(mode))
	if c.registry.DefaultModels != nil {
		if name := cleanModelName(c.registry.DefaultModels[modeKey]); name != "" {
			return name
		}
	}
	legacyDefault := cleanModelName(c.registry.DefaultModel)
	if legacyDefault == "" {
		return ""
	}
	if modeKey == string(component.ModelModeCompletion) {
		return legacyDefault
	}
	if record, ok := c.registry.Models[legacyDefault]; ok && cleanModelMode(record.Mode) == modeKey {
		return legacyDefault
	}
	return ""
}

func (c *Component) resolve(name string, record ModelRecord) component.Model {
	path := strings.TrimSpace(record.Path)
	if path != "" && !filepath.IsAbs(path) {
		path = c.resolveRelativeModelPath(path)
	}
	if path == "" {
		filename := firstNonEmpty(record.Filename, filenameFromURL(record.URL))
		if filename != "" {
			path = filepath.Join(c.config.ModelPath, name, filename)
		}
	}
	return component.Model{
		Name:        cleanModelName(name),
		URL:         strings.TrimSpace(record.URL),
		Filename:    strings.TrimSpace(record.Filename),
		Path:        path,
		Mode:        component.ModelMode(cleanModelMode(record.Mode)),
		SHA256:      strings.TrimSpace(record.SHA256),
		MMProjPath:  strings.TrimSpace(record.MMProjPath),
		HostPort:    record.HostPort,
		ContextSize: record.ContextSize,
		UBatchSize:  record.UBatchSize,
		GPULayers:   record.GPULayers,
		MaxTokens:   record.MaxTokens,
		Temperature: record.Temperature,
		Pooling:     strings.TrimSpace(record.Pooling),
		Normalize:   modelNormalize(record),
	}
}

func (c *Component) resolveRelativeModelPath(path string) string {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "." || path == "" {
		return ""
	}
	if strings.HasPrefix(filepath.ToSlash(path), "models/") {
		legacy := filepath.Join(c.home.Path, path)
		if _, err := os.Stat(legacy); err == nil {
			return legacy
		}
	}
	return filepath.Join(c.config.ModelPath, path)
}

func sortedModelNames(models map[string]ModelRecord) []string {
	names := make([]string, 0, len(models))
	for name := range models {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
