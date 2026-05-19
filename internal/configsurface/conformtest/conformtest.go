// Package conformtest contains reusable tests for configsurface implementations.
package conformtest

import (
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

type Case struct {
	WritableKey      string
	WritableValue    string
	ExpectedSetValue string
	ExpectedUnset    string
	ReadOnlyKey      string
	UnknownKey       string
}

func Assert(t *testing.T, surface configsurface.ConfigSurface, req commandengine.Request, tc Case) {
	t.Helper()
	if surface == nil {
		t.Fatal("missing config surface")
	}
	ctx := context.Background()
	engine := testEngine(t, surface, req.Context.Source)

	list, err := engine.Run(ctx, req, []string{"config", "list"})
	if err != nil {
		t.Fatalf("config list error = %v", err)
	}
	if strings.TrimSpace(list.Text) == "" {
		t.Fatal("config list returned empty output")
	}
	if tc.WritableKey != "" && !strings.Contains(list.Text, configsurface.NormalizeKey(tc.WritableKey)+"=") {
		t.Fatalf("config list missing writable key %q:\n%s", tc.WritableKey, list.Text)
	}
	if tc.ReadOnlyKey != "" && !strings.Contains(list.Text, configsurface.NormalizeKey(tc.ReadOnlyKey)+"=") {
		t.Fatalf("config list missing read-only key %q:\n%s", tc.ReadOnlyKey, list.Text)
	}

	if tc.WritableKey != "" {
		get, err := engine.Run(ctx, req, []string{"config", "get", tc.WritableKey})
		if err != nil {
			t.Fatalf("config get %s error = %v", tc.WritableKey, err)
		}
		for _, want := range []string{configsurface.NormalizeKey(tc.WritableKey) + "=", "writable: true"} {
			if !strings.Contains(get.Text, want) {
				t.Fatalf("config get %s missing %q:\n%s", tc.WritableKey, want, get.Text)
			}
		}

		set, err := engine.Run(ctx, req, []string{"config", "set", tc.WritableKey, tc.WritableValue})
		if err != nil {
			t.Fatalf("config set %s error = %v", tc.WritableKey, err)
		}
		wantSet := configsurface.NormalizeKey(tc.WritableKey) + "=" + tc.ExpectedSetValue
		if strings.TrimSpace(set.Text) != wantSet {
			t.Fatalf("config set %s = %q, want %q", tc.WritableKey, set.Text, wantSet)
		}
	}

	if tc.ReadOnlyKey != "" {
		_, err := engine.Run(ctx, req, []string{"config", "set", tc.ReadOnlyKey, "x"})
		if err == nil || !strings.Contains(err.Error(), "read-only") {
			t.Fatalf("config set read-only %s error = %v, want read-only", tc.ReadOnlyKey, err)
		}
	}

	unknownKey := tc.UnknownKey
	if unknownKey == "" {
		unknownKey = "definitely-unknown-config-key"
	}
	_, err = engine.Run(ctx, req, []string{"config", "get", unknownKey})
	if err == nil || !strings.Contains(err.Error(), "unknown config key") {
		t.Fatalf("config get unknown error = %v, want standard unknown config key", err)
	}

	_, supportsUnset := surface.(configsurface.ConfigUnsetter)
	_, err = engine.Run(ctx, req, []string{"config", "unset", unknownKey})
	if supportsUnset {
		if err == nil || !strings.Contains(err.Error(), "unknown config key") {
			t.Fatalf("config unset unknown error = %v, want standard unknown config key", err)
		}
	} else if err == nil || !strings.Contains(err.Error(), "no matching command") {
		t.Fatalf("config unset without support error = %v, want no matching command", err)
	}

	if supportsUnset && tc.WritableKey != "" {
		unset, err := engine.Run(ctx, req, []string{"config", "unset", tc.WritableKey})
		if err != nil {
			t.Fatalf("config unset %s error = %v", tc.WritableKey, err)
		}
		wantUnset := configsurface.NormalizeKey(tc.WritableKey) + "=" + tc.ExpectedUnset
		if strings.TrimSpace(unset.Text) != wantUnset {
			t.Fatalf("config unset %s = %q, want %q", tc.WritableKey, unset.Text, wantUnset)
		}
	}
}

func testEngine(t *testing.T, surface configsurface.ConfigSurface, source commandengine.Source) *commandengine.Engine {
	t.Helper()
	if source == "" {
		source = commandengine.SourceHostbridge
	}
	_, supportsUnset := surface.(configsurface.ConfigUnsetter)
	router, err := commandengine.NewRouter(configsurface.CommandDefinitions(configsurface.DefinitionOptions{
		Sources:       []commandengine.Source{source},
		Policy:        simplerbac.Public(),
		SupportsUnset: supportsUnset,
	}), source)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	registry := commandengine.NewRegistry()
	if err := configsurface.RegisterCommandHandlers(registry, surface); err != nil {
		t.Fatalf("RegisterCommandHandlers() error = %v", err)
	}
	return commandengine.NewEngine(router, registry)
}
