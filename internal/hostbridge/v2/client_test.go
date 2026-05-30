package v2

import (
	"context"
	"fmt"
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
