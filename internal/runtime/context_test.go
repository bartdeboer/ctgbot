package runtime

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildContextConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	payload := []byte(`{"context":"/host/work","dockerfile":"recipes/Dockerfile","uses":{"Image":"base","Context":"/host/base","Dockerfile":"Dockerfile","Uses":{"Image":"embedded","Dockerfile":"base.Dockerfile"}}}`)
	if err := os.WriteFile(filepath.Join(dir, ConfigFilename), payload, 0600); err != nil {
		t.Fatal(err)
	}
	config, err := LoadBindConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	var decoded BindConfig
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Context != "/host/work" || decoded.Uses.Context != "/host/base" || decoded.Uses.Uses.Context != "" {
		t.Fatalf("config=%s", encoded)
	}
}
