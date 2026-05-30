package v2

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type fakeRunner struct {
	base   commandengine.Request
	argv   []string
	result commandengine.Result
	err    error
}

func (r *fakeRunner) Run(ctx context.Context, base commandengine.Request, argv []string) (commandengine.Result, error) {
	r.base = base
	r.argv = append([]string(nil), argv...)
	return r.result, r.err
}

func TestHandlerMapsPathQueryAndBodyToCommandArgv(t *testing.T) {
	runner := &fakeRunner{result: commandengine.Result{Text: "ok"}}
	handler := NewHandler(runner)

	req := httptest.NewRequest(
		http.MethodPost,
		"/v2/run/indexing/run/search-title?all=true&max-messages=50&dry-run=false",
		strings.NewReader("stdin with `backticks`"),
	)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", rr.Code, http.StatusOK, rr.Body.String())
	}
	want := []string{
		"indexing",
		"run",
		"search-title",
		"--all",
		"--max-messages",
		"50",
		"stdin with `backticks`",
	}
	if got := runner.argv; !equalStrings(got, want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
}

func TestHandlerBuildsContextFromHeaders(t *testing.T) {
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	sandboxID := modeluuid.New()
	runner := &fakeRunner{result: commandengine.Result{Text: "ok"}}
	handler := NewHandler(runner)

	req := httptest.NewRequest(http.MethodPost, "/v2/run/status", nil)
	req.Header.Set("X-Actor-Id", "agent-1")
	req.Header.Set("X-Chat-Id", chatID.String())
	req.Header.Set("X-Thread-Id", threadID.String())
	req.Header.Set("X-Sandbox-Id", sandboxID.String())
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", rr.Code, http.StatusOK, rr.Body.String())
	}
	ctx := runner.base.Context
	if ctx.Source != commandengine.SourceHostbridge {
		t.Fatalf("source = %q, want %q", ctx.Source, commandengine.SourceHostbridge)
	}
	if ctx.Actor.ID != "agent-1" {
		t.Fatalf("actor id = %q, want %q", ctx.Actor.ID, "agent-1")
	}
	if ctx.ChatID != chatID || ctx.ThreadID != threadID || ctx.SandboxID != sandboxID {
		t.Fatalf("context ids = chat:%s thread:%s sandbox:%s", ctx.ChatID, ctx.ThreadID, ctx.SandboxID)
	}
}

func TestHandlerSourceCanBeConfigured(t *testing.T) {
	runner := &fakeRunner{result: commandengine.Result{Text: "ok"}}
	handler := NewHandler(runner)
	handler.Source = commandengine.SourceScheduler

	req := httptest.NewRequest(http.MethodPost, "/v2/run/status", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := runner.base.Context.Source; got != commandengine.SourceScheduler {
		t.Fatalf("source = %q, want %q", got, commandengine.SourceScheduler)
	}
}

func TestHandlerReturnsPlainTextByDefault(t *testing.T) {
	runner := &fakeRunner{result: commandengine.Result{Text: "plain output\n"}}
	handler := NewHandler(runner)

	req := httptest.NewRequest(http.MethodPost, "/v2/run/status", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got, want := rr.Body.String(), "plain output\n"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
	if got := rr.Header().Get("X-Command-Exit-Code"); got != "0" {
		t.Fatalf("exit header = %q, want 0", got)
	}
}

func TestHandlerReturnsJSONWhenRequested(t *testing.T) {
	runner := &fakeRunner{result: commandengine.Result{Text: "json output"}}
	handler := NewHandler(runner)

	req := httptest.NewRequest(http.MethodPost, "/v2/run/status", nil)
	req.Header.Set("Accept", "application/json")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	var resp JSONResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode JSON response: %v", err)
	}
	if resp.ExitCode != 0 || resp.Stdout != "json output" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestHandlerReportsCommandErrorAsCommandFailure(t *testing.T) {
	runner := &fakeRunner{err: errors.New("boom")}
	handler := NewHandler(runner)

	req := httptest.NewRequest(http.MethodPost, "/v2/run/status", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusOK)
	}
	if got, want := rr.Header().Get("X-Command-Exit-Code"), "1"; got != want {
		t.Fatalf("exit header = %q, want %q", got, want)
	}
	if got, want := rr.Body.String(), "boom"; got != want {
		t.Fatalf("body = %q, want %q", got, want)
	}
}

func TestHandlerRejectsNonPost(t *testing.T) {
	runner := &fakeRunner{result: commandengine.Result{Text: "ok"}}
	handler := NewHandler(runner)

	req := httptest.NewRequest(http.MethodGet, "/v2/run/status", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func equalStrings(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
