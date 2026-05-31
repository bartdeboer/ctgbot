package v2

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
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
	base       commandengine.Request
	argv       []string
	helpBase   commandengine.Request
	helpScope  []string
	result     commandengine.Result
	helpResult commandengine.Result
	err        error
	helpErr    error
}

func (r *fakeRunner) Run(ctx context.Context, base commandengine.Request, argv []string) (commandengine.Result, error) {
	r.base = base
	r.argv = append([]string(nil), argv...)
	return r.result, r.err
}

func (r *fakeRunner) Help(ctx context.Context, base commandengine.Request, scope []string) (commandengine.Result, error) {
	r.helpBase = base
	r.helpScope = append([]string(nil), scope...)
	return r.helpResult, r.helpErr
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
	}
	if got := runner.argv; !equalStrings(got, want) {
		t.Fatalf("argv = %#v, want %#v", got, want)
	}
	if got, want := runner.base.Stdin, "stdin with `backticks`"; got != want {
		t.Fatalf("stdin = %q, want %q", got, want)
	}
}

func TestHandlerRendersHelpBeforeRouteParsing(t *testing.T) {
	runner := &fakeRunner{helpResult: commandengine.Result{Text: "send help"}}
	handler := NewHandler(runner)

	req := httptest.NewRequest(http.MethodPost, "/v2/run/send/--help", strings.NewReader("not argv"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body=%q", rr.Code, http.StatusOK, rr.Body.String())
	}
	if got := rr.Body.String(); got != "send help" {
		t.Fatalf("body = %q, want help", got)
	}
	if runner.argv != nil {
		t.Fatalf("run argv = %#v, want help path to bypass command execution", runner.argv)
	}
	if got, want := runner.helpScope, []string{"send"}; !equalStrings(got, want) {
		t.Fatalf("help scope = %#v, want %#v", got, want)
	}
	if got, want := runner.helpBase.Stdin, "not argv"; got != want {
		t.Fatalf("stdin = %q, want %q", got, want)
	}
}

func TestHandlerBuildsContextFromHeaders(t *testing.T) {
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	sandboxID := modeluuid.New()
	runner := &fakeRunner{result: commandengine.Result{Text: "ok"}}
	handler := NewHandler(runner)
	handler.Auth = StaticActorAuth{Actor: commandengine.Actor{ID: "agent-1"}}

	req := httptest.NewRequest(http.MethodPost, "/v2/run/status", nil)
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

func TestNewServerAppliesSharedHTTPContracts(t *testing.T) {
	runner := &fakeRunner{result: commandengine.Result{Text: "ok"}}
	server := NewServer(runner, ServerConfig{
		Addr:   "127.0.0.1:0",
		Source: commandengine.SourceRemoteHostbridge,
		Auth:   StaticActorAuth{Actor: commandengine.Actor{ID: "remote-agent"}},
	})

	handler, ok := server.Handler.(*Handler)
	if !ok {
		t.Fatalf("server handler = %T, want *Handler", server.Handler)
	}
	if server.Addr != "127.0.0.1:0" {
		t.Fatalf("server addr = %q", server.Addr)
	}
	if handler.Runner != runner {
		t.Fatalf("handler runner not preserved")
	}
	if handler.Source != commandengine.SourceRemoteHostbridge {
		t.Fatalf("source = %q, want %q", handler.Source, commandengine.SourceRemoteHostbridge)
	}
}

func TestMTLSClientAuthUsesCertificateCommonName(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/v2/run/status", nil)
	req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{
		{Subject: pkix.Name{CommonName: "thread-1"}},
	}}

	actor, err := (MTLSClientAuth{}).Authenticate(req)
	if err != nil {
		t.Fatalf("authenticate mTLS: %v", err)
	}
	if actor.ID != "thread-1" {
		t.Fatalf("actor id = %q, want thread-1", actor.ID)
	}
}

func TestBearerTokenAuthRequiresAuthorizationHeader(t *testing.T) {
	auth := BearerTokenAuth{
		Token: "secret",
		Actor: commandengine.Actor{ID: "remote-agent"},
	}
	req := httptest.NewRequest(http.MethodPost, "/v2/run/status?access_token=secret", nil)

	if _, err := auth.Authenticate(req); err == nil {
		t.Fatalf("Authenticate accepted bearer token in query")
	}

	req = httptest.NewRequest(http.MethodPost, "/v2/run/status", nil)
	req.Header.Set("Authorization", "Bearer secret")
	actor, err := auth.Authenticate(req)
	if err != nil {
		t.Fatalf("authenticate bearer token: %v", err)
	}
	if actor.ID != "remote-agent" {
		t.Fatalf("actor id = %q, want remote-agent", actor.ID)
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
