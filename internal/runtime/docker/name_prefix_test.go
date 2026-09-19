package docker

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
)

func TestContainerNamePrefixFromProfile(t *testing.T) {
	thread, err := modeluuid.Parse("00VGyvELR38v6AqboKyD48w")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, config, prefix string }{
		{"omitted", `{}`, ""},
		{"empty", `{"container_name_prefix":"  "}`, ""},
		{"custom", `{"container_name_prefix":" work- "}`, "work-"},
		{"full-prefix", `{"container_name_prefix":"work-codex-"}`, "work-codex-"},
		{"docker-characters", `{"container_name_prefix":"Work_1.dev"}`, "Work_1.dev"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, runtimepkg.ConfigFilename), []byte(tc.config), 0600); err != nil {
				t.Fatal(err)
			}
			config, err := runtimepkg.LoadBindConfig(root)
			if err != nil {
				t.Fatal(err)
			}
			factory := New(root, filepath.Join(root, "components"), fakeSandboxManager{}, nil)
			for _, registration := range []coremodel.Component{{Type: "codex", Name: "codex"}, {Type: "codex", Name: "work"}, {Type: "claude", Name: "claude"}} {
				rt := factory.Bind(registration, runtimepkg.Profile{Path: root}, config).(*Runtime)
				workspace := filepath.Join(root, "workspace")
				sbx, err := rt.ThreadSandbox(t.Context(), workspace, thread)
				if err != nil {
					t.Fatal(err)
				}
				want := tc.prefix + thread.String()
				wantAuth := tc.prefix + "auth"
				if tc.prefix == "" {
					want = "ctgbot-" + safeName(registration.Ref(), "") + "-" + thread.String()
					wantAuth = "ctgbot-auth-" + safeName(registration.Ref(), "")
				}
				if sbx.Name != want || sbx.Hostname != want {
					t.Fatalf("name=%q hostname=%q want=%q", sbx.Name, sbx.Hostname, want)
				}
				if sbx.HomeDir != filepath.Join(root, "threads", thread.String(), "home") || sbx.ProfileDir != root {
					t.Fatal("prefix changed durable paths")
				}
				auth, err := rt.ThreadSandbox(t.Context(), "", modeluuid.UUID{})
				if err != nil {
					t.Fatal(err)
				}
				if auth.Name != wantAuth {
					t.Fatalf("auth=%q want=%q", auth.Name, wantAuth)
				}
			}
		})
	}
}

func TestInvalidContainerNamePrefixBeforeSandboxCreation(t *testing.T) {
	for _, prefix := range []string{"../work", "a/b", "two words", "-work", "bad\x00prefix"} {
		t.Run(prefix, func(t *testing.T) {
			// Unimplemented manager methods panic if validation reaches sandbox creation.
			rt := New(t.TempDir(), "", nil, nil).Bind(coremodel.Component{Type: "codex", Name: "codex"}, runtimepkg.Profile{}, runtimepkg.BindConfig{ContainerNamePrefix: prefix}).(*Runtime)
			rt.sandboxes = forbiddenPrefixManager{}
			for _, id := range []modeluuid.UUID{{}, modeluuid.New()} {
				if _, err := rt.ThreadSandbox(t.Context(), "unused", id); err == nil {
					t.Fatal("invalid prefix accepted")
				}
			}
		})
	}
}

type forbiddenPrefixManager struct{ sandboxengine.RuntimeManager }
