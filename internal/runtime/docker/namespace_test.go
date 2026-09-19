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

func TestContainerNamespaceFromProfile(t *testing.T) {
	thread, err := modeluuid.Parse("00VGyvELR38v6AqboKyD48w")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, config, namespace string }{
		{"omitted", `{}`, ""},
		{"empty", `{"container_namespace":"  "}`, ""},
		{"custom", `{"container_namespace":" work "}`, "work"},
		{"component-namespace", `{"container_namespace":"work-codex"}`, "work-codex"},
		{"docker-characters", `{"container_namespace":"Work_1.dev"}`, "Work_1.dev"},
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
				want := "ctgbot-" + tc.namespace + "-" + thread.String()
				wantAuth := "ctgbot-" + tc.namespace + "-auth"
				if tc.namespace == "" {
					want = "ctgbot-" + safeName(registration.Ref(), "") + "-" + thread.String()
					wantAuth = "ctgbot-auth-" + safeName(registration.Ref(), "")
				}
				if sbx.Name != want || sbx.Hostname != want {
					t.Fatalf("name=%q hostname=%q want=%q", sbx.Name, sbx.Hostname, want)
				}
				if sbx.HomeDir != filepath.Join(root, "threads", thread.String(), "home") || sbx.ProfileDir != root {
					t.Fatal("namespace changed durable paths")
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

func TestInvalidContainerNamespaceBeforeSandboxCreation(t *testing.T) {
	for _, namespace := range []string{"../work", "a/b", "two words", "-work", "bad\x00namespace"} {
		t.Run(namespace, func(t *testing.T) {
			// Unimplemented manager methods panic if validation reaches sandbox creation.
			rt := New(t.TempDir(), "", nil, nil).Bind(coremodel.Component{Type: "codex", Name: "codex"}, runtimepkg.Profile{}, runtimepkg.BindConfig{ContainerNamespace: namespace}).(*Runtime)
			rt.sandboxes = forbiddenNamespaceManager{}
			for _, id := range []modeluuid.UUID{{}, modeluuid.New()} {
				if _, err := rt.ThreadSandbox(t.Context(), "unused", id); err == nil {
					t.Fatal("invalid namespace accepted")
				}
			}
		})
	}
}

type forbiddenNamespaceManager struct{ sandboxengine.RuntimeManager }
