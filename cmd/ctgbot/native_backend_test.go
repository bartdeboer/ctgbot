package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/runtime/backend"
	"github.com/bartdeboer/go-clistate"
)

func TestResidentNativeHelper(t *testing.T) {
	if os.Getenv("CTGBOT_RESIDENT_HELPER") != "1" {
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("ok")) })
	if http.ListenAndServe(os.Getenv("HELPER_ADDR"), mux) != nil {
		os.Exit(19)
	}
	os.Exit(0)
}

// Exercise actual CLI composition and the exact ownership seam used by run.
func TestNativeCLIAndResidentComposition(t *testing.T) {
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(oldwd) })
	root := t.TempDir()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatal(err)
	}
	profile := t.TempDir()
	modelPath := filepath.Join(profile, "dummy.gguf")
	os.WriteFile(modelPath, []byte("not a model; never opened"), 0600)
	runtimeJSON := `{"driver":"native","native":{"executable":"/must-not-execute"}}`
	os.WriteFile(filepath.Join(profile, "runtime.json"), []byte(runtimeJSON), 0600)
	data, _ := json.Marshal(map[string]any{"model_path": modelPath})
	os.WriteFile(filepath.Join(profile, "component.json"), data, 0600)
	var output bytes.Buffer
	if err := runCLICommand(context.Background(), []string{"component", "register", "llamacpp/native-test", "--runtime", "backend", "--profile", profile}, store, nil, &output); err != nil {
		t.Fatal(err)
	}
	for _, verb := range []string{"start", "stop", "status"} {
		output.Reset()
		err := runCLICommand(context.Background(), []string{"component", "llamacpp/native-test", verb}, store, nil, &output)
		if err == nil || !strings.Contains(err.Error(), "resident ctgbot run") {
			t.Fatalf("%s: %v / %s", verb, err, output.String())
		}
	}
	output.Reset()
	if err := runCLICommand(context.Background(), []string{"component", "llamacpp/native-test", "help"}, store, nil, &output); err != nil {
		t.Fatal(err)
	}

	s, err := openSystem(context.Background(), store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	closeOwner, err := ownNativeBackends(s, func(string, ...any) {})
	if err != nil {
		t.Fatal(err)
	}
	defer closeOwner()
	f := s.Runtimes["backend"].(*backend.Factory)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	listener.Close()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	service, err := f.BindBackend(coremodel.Component{}, runtimepkg.Profile{Path: profile}, runtimepkg.BindConfig{}, backend.ServiceSpec{
		Identity: "composition-helper", Native: &backend.NativeConfig{Executable: executable, Args: []string{"-test.run=^TestResidentNativeHelper$"}},
		Env: []string{"CTGBOT_RESIDENT_HELPER=1", "HELPER_ADDR=" + addr}, BaseURL: "http://" + addr, HealthURL: "http://" + addr + "/health",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	closeOwner()
	if status, err := service.Status(context.Background()); err != nil || status.State != "stopped" {
		t.Fatal(status, err)
	}
	if _, err := service.Start(context.Background()); err == nil {
		t.Fatal("closed resident reopened")
	}
	if _, err := ownNativeBackends(s, func(string, ...any) {}); err == nil {
		t.Fatal("ownership reopened after shutdown")
	}
}
