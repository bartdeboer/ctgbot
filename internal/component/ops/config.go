package ops

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/go-clir"
)

type ConfigStore interface {
	RootDir() string
	ResolveStruct(key string, out any) (source fmt.Stringer, ok bool, err error)
	PersistOverlayStruct(layerName, key string, val any) error
	UnsetOverlay(layerName, key string) error
}

type configGetCommand struct {
	Key string
}

type configSetCommand struct {
	Layer string
	Key   string
	Value string
}

type configUnsetCommand struct {
	Layer string
	Key   string
}

type configLayersCommand struct{}

func buildConfigGet(req *clir.Request) (any, error) {
	key := strings.TrimSpace(req.Params["key"])
	if key == "" {
		return nil, fmt.Errorf("missing config key")
	}
	return configGetCommand{Key: key}, nil
}

func buildConfigSet(req *clir.Request) (any, error) {
	layer := strings.TrimSpace(req.Params["layer"])
	key := strings.TrimSpace(req.Params["key"])
	if layer == "" {
		return nil, fmt.Errorf("missing config layer")
	}
	if key == "" {
		return nil, fmt.Errorf("missing config key")
	}
	return configSetCommand{Layer: layer, Key: key, Value: req.Params["value"]}, nil
}

func buildConfigUnset(req *clir.Request) (any, error) {
	layer := strings.TrimSpace(req.Params["layer"])
	key := strings.TrimSpace(req.Params["key"])
	if layer == "" {
		return nil, fmt.Errorf("missing config layer")
	}
	if key == "" {
		return nil, fmt.Errorf("missing config key")
	}
	return configUnsetCommand{Layer: layer, Key: key}, nil
}

func (c *Component) handleConfigGet(ctx context.Context, req commandengine.Request, cmd configGetCommand) (commandengine.Result, error) {
	_, _ = ctx, req
	var value any
	source, ok, err := c.configForUse().ResolveStruct(cmd.Key, &value)
	if err != nil {
		return commandengine.Result{}, err
	}
	if !ok {
		return commandengine.Result{Text: fmt.Sprintf("ops config\n%s: not set", cmd.Key)}, nil
	}
	formatted, err := formatConfigValue(value)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: strings.Join([]string{"ops config", fmt.Sprintf("key: %s", cmd.Key), fmt.Sprintf("source: %s", source.String()), "value:", formatted}, "\n")}, nil
}

func (c *Component) handleConfigSet(ctx context.Context, req commandengine.Request, cmd configSetCommand) (commandengine.Result, error) {
	_, _ = ctx, req
	value, err := parseConfigValue(cmd.Value)
	if err != nil {
		return commandengine.Result{}, err
	}
	if err := c.configForUse().PersistOverlayStruct(cmd.Layer, cmd.Key, value); err != nil {
		return commandengine.Result{}, err
	}
	formatted, err := formatConfigValue(value)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: strings.Join([]string{"ops config set", fmt.Sprintf("layer: %s", cmd.Layer), fmt.Sprintf("key: %s", cmd.Key), "value:", formatted}, "\n")}, nil
}

func (c *Component) handleConfigUnset(ctx context.Context, req commandengine.Request, cmd configUnsetCommand) (commandengine.Result, error) {
	_, _ = ctx, req
	if err := c.configForUse().UnsetOverlay(cmd.Layer, cmd.Key); err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: strings.Join([]string{"ops config unset", fmt.Sprintf("layer: %s", cmd.Layer), fmt.Sprintf("key: %s", cmd.Key)}, "\n")}, nil
}

func (c *Component) handleConfigLayers(ctx context.Context, req commandengine.Request, cmd configLayersCommand) (commandengine.Result, error) {
	_, _, _ = ctx, req, cmd
	layers, err := listConfigLayers(c.configForUse())
	if err != nil {
		return commandengine.Result{}, err
	}
	if len(layers) == 0 {
		return commandengine.Result{Text: "ops config layers\n(no config.d layers)"}, nil
	}
	return commandengine.Result{Text: "ops config layers\n" + strings.Join(layers, "\n")}, nil
}

func parseConfigValue(raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err == nil {
		return value, nil
	}
	return raw, nil
}

func formatConfigValue(value any) (string, error) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func listConfigLayers(cfg ConfigStore) ([]string, error) {
	if cfg == nil {
		return nil, fmt.Errorf("missing ops config store")
	}
	dir := filepath.Join(cfg.RootDir(), "config.d")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var layers []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		layers = append(layers, entry.Name())
	}
	sort.Strings(layers)
	return layers, nil
}

func (c *Component) configForUse() ConfigStore {
	if c == nil || c.config == nil {
		return missingConfigStore{}
	}
	return c.config
}

type missingConfigStore struct{}

func (missingConfigStore) RootDir() string { return "" }
func (missingConfigStore) ResolveStruct(string, any) (fmt.Stringer, bool, error) {
	return nil, false, fmt.Errorf("missing ops config store")
}
func (missingConfigStore) PersistOverlayStruct(string, string, any) error {
	return fmt.Errorf("missing ops config store")
}
func (missingConfigStore) UnsetOverlay(string, string) error {
	return fmt.Errorf("missing ops config store")
}
