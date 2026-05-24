package llamacppagent

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/toolloop"
)

type toolloopRunFiles struct {
	HostDir     string
	RuntimeDir  string
	RequestHost string
	ResultHost  string
	EventsHost  string
}

func newToolloopRunFiles(hostHome string, runtimeHome string, threadID modeluuid.UUID) (*toolloopRunFiles, error) {
	runName := threadID.String() + "-" + modeluuid.New().String()
	hostDir := filepath.Join(hostHome, "toolloop", runName)
	if err := os.MkdirAll(hostDir, 0o700); err != nil {
		return nil, err
	}
	runtimeDir := filepath.ToSlash(filepath.Join(runtimeHome, "toolloop", runName))
	return &toolloopRunFiles{
		HostDir:     hostDir,
		RuntimeDir:  runtimeDir,
		RequestHost: filepath.Join(hostDir, "request.json"),
		ResultHost:  filepath.Join(hostDir, "result.json"),
		EventsHost:  filepath.Join(hostDir, "events.jsonl"),
	}, nil
}

func (f *toolloopRunFiles) Cleanup() {
	if f != nil {
		_ = os.RemoveAll(f.HostDir)
	}
}

func (f *toolloopRunFiles) WriteRequest(req toolloop.Request) error {
	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(f.RequestHost, data, 0o600)
}

func (f *toolloopRunFiles) RequestRuntime() string {
	return filepath.ToSlash(filepath.Join(f.RuntimeDir, "request.json"))
}
func (f *toolloopRunFiles) ResultRuntime() string {
	return filepath.ToSlash(filepath.Join(f.RuntimeDir, "result.json"))
}
func (f *toolloopRunFiles) EventsRuntime() string {
	return filepath.ToSlash(filepath.Join(f.RuntimeDir, "events.jsonl"))
}

func (f *toolloopRunFiles) DebugFiles() toolloop.DebugFiles {
	return toolloop.DebugFiles{Request: f.RequestHost, Result: f.ResultHost, Events: f.EventsHost}
}
