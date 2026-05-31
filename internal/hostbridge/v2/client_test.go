package v2

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type e2eEchoCommand struct {
	Text string
}

type e2eStreamCommand struct {
	Text string
}

func TestClientRunsCommandThroughHTTPProtocol(t *testing.T) {
	chatID := modeluuid.New()
	threadID := modeluuid.New()
	sandboxID := modeluuid.New()
	engine := newTestEngine(t, commandengine.SourceHostbridge, func(ctx context.Context, req commandengine.Request, cmd e2eEchoCommand) (commandengine.Result, error) {
		if req.Context.ChatID != chatID || req.Context.ThreadID != threadID || req.Context.SandboxID != sandboxID {
			return commandengine.Result{}, fmt.Errorf("context ids were not forwarded")
		}
		return commandengine.Result{Text: "echo:" + cmd.Text}, nil
	})
	server := httptest.NewServer(NewServer(engine, ServerConfig{
		Source: commandengine.SourceHostbridge,
		Auth:   StaticActorAuth{Actor: commandengine.Actor{ID: "agent-1", Roles: []simplerbac.Role{simplerbac.RoleAgent}}},
	}).Handler)
	defer server.Close()

	client := &Client{BaseURL: server.URL}
	resp, err := client.Run(context.Background(), RunRequest{
		Command:   []string{"echo"},
		Query:     url.Values{"loud": []string{"false"}},
		Stdin:     "hello with `backticks`",
		ChatID:    chatID,
		ThreadID:  threadID,
		SandboxID: sandboxID,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.StatusCode != 200 || resp.ExitCode != 0 || resp.Text != "echo:hello with `backticks`" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestClientRequestsJSONResponse(t *testing.T) {
	engine := newTestEngine(t, commandengine.SourceHostbridge, func(ctx context.Context, req commandengine.Request, cmd e2eEchoCommand) (commandengine.Result, error) {
		return commandengine.Result{Text: "json:" + cmd.Text}, nil
	})
	server := httptest.NewServer(NewServer(engine, ServerConfig{
		Source: commandengine.SourceHostbridge,
		Auth:   StaticActorAuth{Actor: commandengine.Actor{ID: "agent-1", Roles: []simplerbac.Role{simplerbac.RoleAgent}}},
	}).Handler)
	defer server.Close()

	client := &Client{BaseURL: server.URL}
	resp, err := client.Run(context.Background(), RunRequest{
		Command:  []string{"echo"},
		Stdin:    "hello",
		WantJSON: true,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.JSON.ExitCode != 0 || resp.JSON.Stdout != "json:hello" || resp.Text != "json:hello" {
		t.Fatalf("response = %+v", resp)
	}
}

func TestClientUsesBearerTokenForRemoteHTTP(t *testing.T) {
	engine := newTestEngine(t, commandengine.SourceRemoteHostbridge, func(ctx context.Context, req commandengine.Request, cmd e2eEchoCommand) (commandengine.Result, error) {
		if req.Context.Source != commandengine.SourceRemoteHostbridge {
			return commandengine.Result{}, fmt.Errorf("source = %q", req.Context.Source)
		}
		return commandengine.Result{Text: "remote:" + cmd.Text}, nil
	})
	server := httptest.NewServer(NewServer(engine, ServerConfig{
		Source: commandengine.SourceRemoteHostbridge,
		Auth: BearerTokenAuth{
			Token: "secret",
			Actor: commandengine.Actor{ID: "remote-agent", Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		},
	}).Handler)
	defer server.Close()

	client := &Client{BaseURL: server.URL, BearerToken: "secret"}
	resp, err := client.Run(context.Background(), RunRequest{Command: []string{"echo"}, Stdin: "hello"})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if resp.Text != "remote:hello" {
		t.Fatalf("text = %q", resp.Text)
	}

	client.BearerToken = ""
	resp, err = client.Run(context.Background(), RunRequest{Command: []string{"echo"}, Stdin: "hello"})
	if err == nil || resp.StatusCode != 401 {
		t.Fatalf("missing token Run() = resp:%+v err:%v, want 401 error", resp, err)
	}
}

func TestHandlerStreamsCommandOutputAsSSE(t *testing.T) {
	engine := newStreamTestEngine(t, commandengine.SourceHostbridge, func(ctx context.Context, req commandengine.Request, cmd e2eStreamCommand) (commandengine.Result, error) {
		if req.OutputStream == nil {
			return commandengine.Result{}, fmt.Errorf("missing output stream")
		}
		req.OutputStream.Stdout("out:" + cmd.Text)
		req.OutputStream.Stderr("warn:" + cmd.Text)
		req.OutputStream.Event("progress", map[string]any{"step": 1})
		return commandengine.Result{Text: "done:" + cmd.Text}, nil
	})
	server := httptest.NewServer(NewServer(engine, ServerConfig{
		Source: commandengine.SourceHostbridge,
		Auth:   StaticActorAuth{Actor: commandengine.Actor{ID: "agent-1", Roles: []simplerbac.Role{simplerbac.RoleAgent}}},
	}).Handler)
	defer server.Close()

	httpReq, err := http.NewRequest(http.MethodPost, server.URL+"/v2/run/stream/hello", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer httpResp.Body.Close()
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"event: started\n",
		"event: stdout\n",
		`"text":"out:hello"`,
		"event: stderr\n",
		`"text":"warn:hello"`,
		"event: event\n",
		`"kind":"progress"`,
		"event: completed\n",
		`"summary":"done:hello"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("SSE body missing %q:\n%s", want, text)
		}
	}
}

func TestHandlerStreamsCommandFailureAsSSE(t *testing.T) {
	engine := newStreamTestEngine(t, commandengine.SourceHostbridge, func(ctx context.Context, req commandengine.Request, cmd e2eStreamCommand) (commandengine.Result, error) {
		req.OutputStream.Stdout("before failure")
		return commandengine.Result{}, fmt.Errorf("failed %s", cmd.Text)
	})
	server := httptest.NewServer(NewServer(engine, ServerConfig{
		Source: commandengine.SourceHostbridge,
		Auth:   StaticActorAuth{Actor: commandengine.Actor{ID: "agent-1", Roles: []simplerbac.Role{simplerbac.RoleAgent}}},
	}).Handler)
	defer server.Close()

	httpReq, err := http.NewRequest(http.MethodPost, server.URL+"/v2/run/stream/oops", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	httpReq.Header.Set("Accept", "text/event-stream")
	httpResp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer httpResp.Body.Close()
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	text := string(body)
	for _, want := range []string{
		"event: stdout\n",
		`"text":"before failure"`,
		"event: failed\n",
		`"error":"failed oops"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("SSE body missing %q:\n%s", want, text)
		}
	}
}

func TestClientEscapesCommandPathSegments(t *testing.T) {
	client := &Client{BaseURL: "https://example.test/root"}
	target, err := client.runURL(RunRequest{
		Command: []string{"thread", "00 abc/def", "message", "send"},
		Query:   url.Values{"dry-run": []string{"true"}},
	})
	if err != nil {
		t.Fatalf("runURL() error = %v", err)
	}
	if !strings.Contains(target, "/root/v2/run/thread/00%20abc%2Fdef/message/send") {
		t.Fatalf("target URL = %q", target)
	}
	if !strings.Contains(target, "dry-run=true") {
		t.Fatalf("target URL missing query: %q", target)
	}
}

func newStreamTestEngine(
	t *testing.T,
	source commandengine.Source,
	handler commandengine.HandlerFunc[e2eStreamCommand],
) *commandengine.Engine {
	t.Helper()
	definitions := []commandengine.Definition{
		{
			Pattern: "stream <text>",
			Sources: []commandengine.Source{source},
			Policy:  simplerbac.Any(simplerbac.RoleAgent),
			Build: func(req *clir.Request) (any, error) {
				return e2eStreamCommand{Text: req.Params["text"]}, nil
			},
		},
	}
	router, err := commandengine.NewRouter(definitions, source)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	registry := commandengine.NewRegistry()
	if err := commandengine.Register[e2eStreamCommand](registry, handler); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	return commandengine.NewEngine(router, registry)
}

func newTestEngine(
	t *testing.T,
	source commandengine.Source,
	handler commandengine.HandlerFunc[e2eEchoCommand],
) *commandengine.Engine {
	t.Helper()
	definitions := []commandengine.Definition{
		{
			Pattern: "echo <text>",
			Sources: []commandengine.Source{source},
			Policy:  simplerbac.Any(simplerbac.RoleAgent),
			Build: func(req *clir.Request) (any, error) {
				return e2eEchoCommand{Text: req.Params["text"]}, nil
			},
		},
	}
	router, err := commandengine.NewRouter(definitions, source)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	registry := commandengine.NewRegistry()
	if err := commandengine.Register[e2eEchoCommand](registry, handler); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	return commandengine.NewEngine(router, registry)
}
