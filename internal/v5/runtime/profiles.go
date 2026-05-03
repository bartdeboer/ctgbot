package runtime

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/go-clistate"
)

const profileConfigKey = "v5.profiles"

type ProfileSettings struct {
	Runtime  string `json:"runtime"`
	HomePath string `json:"home_path"`
}

func ConfiguredProfiles(store *clistate.Store) map[string]ProfileSettings {
	if store == nil {
		return map[string]ProfileSettings{}
	}
	var out map[string]ProfileSettings
	store.GetStruct(profileConfigKey, &out)
	if out == nil {
		return map[string]ProfileSettings{}
	}
	return out
}

func LoadProfiles(rootDir string, store *clistate.Store) (map[string]component.Profile, error) {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return nil, fmt.Errorf("missing root dir")
	}
	configured := ConfiguredProfiles(store)
	profiles := map[string]component.Profile{}
	for name, settings := range configured {
		profile, err := resolveProfile(rootDir, name, settings)
		if err != nil {
			return nil, err
		}
		profiles[name] = profile
	}
	if _, ok := profiles["default"]; !ok {
		profile, err := resolveProfile(rootDir, "default", ProfileSettings{})
		if err != nil {
			return nil, err
		}
		profiles["default"] = profile
	}
	return profiles, nil
}

func SaveProfile(rootDir string, store *clistate.Store, name string, runtimeKind string, homePath string) (component.Profile, error) {
	if store == nil {
		return component.Profile{}, fmt.Errorf("missing profile store")
	}
	name = strings.TrimSpace(name)
	if err := validateProfileName(name); err != nil {
		return component.Profile{}, err
	}
	runtimeKind = strings.TrimSpace(runtimeKind)
	homePath = strings.TrimSpace(homePath)

	configured := ConfiguredProfiles(store)
	settings := configured[name]
	if runtimeKind != "" {
		settings.Runtime = runtimeKind
	}
	settings.HomePath = homePath
	configured[name] = settings
	if err := store.PersistStruct(profileConfigKey, configured); err != nil {
		return component.Profile{}, err
	}
	return resolveProfile(rootDir, name, settings)
}

func resolveProfile(rootDir string, name string, settings ProfileSettings) (component.Profile, error) {
	if err := validateProfileName(name); err != nil {
		return component.Profile{}, err
	}
	runtimeKind := strings.TrimSpace(settings.Runtime)
	if runtimeKind == "" {
		runtimeKind = "docker"
	}
	root, err := resolveProfileRoot(rootDir, name, strings.TrimSpace(settings.HomePath))
	if err != nil {
		return component.Profile{}, err
	}
	return component.Profile{
		Name:    name,
		Runtime: runtimeKind,
		Root:    root,
	}, nil
}

func resolveProfileRoot(rootDir string, name string, homePath string) (string, error) {
	rootDir = strings.TrimSpace(rootDir)
	if rootDir == "" {
		return "", fmt.Errorf("missing root dir")
	}
	switch {
	case homePath == "":
		return filepath.Join(rootDir, ".ctgbot", "profiles", name), nil
	case filepath.IsAbs(homePath):
		return filepath.Clean(homePath), nil
	default:
		return filepath.Clean(filepath.Join(rootDir, homePath)), nil
	}
}

func validateProfileName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("missing profile name")
	}
	if name == "." || name == ".." {
		return fmt.Errorf("invalid profile name: %q", name)
	}
	if strings.ContainsAny(name, `/\\`) {
		return fmt.Errorf("invalid profile name: %q", name)
	}
	if filepath.Clean(name) != name {
		return fmt.Errorf("invalid profile name: %q", name)
	}
	return nil
}
