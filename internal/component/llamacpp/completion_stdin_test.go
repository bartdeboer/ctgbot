package llamacpp

import (
	"bytes"
	"context"
	"encoding/gob"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	backendruntime "github.com/bartdeboer/ctgbot/internal/runtime/backend"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/ctgbot/internal/workgate"
)

type stdinBackend struct {
	runtimepkg.ServiceRuntime
	url    string
	starts int
}

func (b *stdinBackend) BaseURL() string { return b.url }
func (b *stdinBackend) Start(context.Context) (runtimepkg.Status, error) {
	b.starts++
	return runtimepkg.Status{State: "running"}, nil
}
func (b *stdinBackend) Status(context.Context) (runtimepkg.Status, error) {
	return runtimepkg.Status{State: "running"}, nil
}

type stdinFactory struct {
	backendruntime.Binder
	b *stdinBackend
}

func (f stdinFactory) BindBackend(coremodel.Component, runtimepkg.Profile, runtimepkg.BindConfig, backendruntime.ServiceSpec) (runtimepkg.ServiceRuntime, error) {
	return f.b, nil
}

func TestStdinCompletionTypedHandlerAndBackend(t *testing.T) {
	gob.Register(completionCommand{})
	document := "  \"hello\"\nРусский\\Dutch\n "
	var messages []chatMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Messages []chatMessage }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		messages = body.Messages
		_, _ = fmt.Fprint(w, `{"choices":[{"message":{"content":"answer"}}]}`)
	}))
	defer server.Close()
	backend := &stdinBackend{url: server.URL}
	c := &Component{componentConfig: ComponentConfig{ModelPath: "/unused.gguf"}.withDefaults(), backendFactory: stdinFactory{b: backend}, client: server.Client(), inferenceGate: workgate.New()}
	router, err := commandengine.NewRouter(c.CommandDefinitions(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatal(err)
	}
	registry := commandengine.NewRegistry()
	if err := c.RegisterCommandHandlers(registry); err != nil {
		t.Fatal(err)
	}
	for _, stdin := range []bool{true, false} {
		args := []string{"completion", "Summarize"}
		if stdin {
			args = append(args, "--stdin")
		}
		req, err := router.Parse(t.Context(), commandengine.Request{Context: commandengine.Context{Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}}}}, args)
		if err != nil {
			t.Fatal(err)
		}
		if stdin {
			req.Stdin = document
		}
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(req); err != nil {
			t.Fatal(err)
		}
		var decoded commandengine.Request
		if err := gob.NewDecoder(&buf).Decode(&decoded); err != nil {
			t.Fatal(err)
		}
		result, err := registry.Execute(t.Context(), decoded)
		if err != nil || result.Text != "answer" {
			t.Fatalf("result=%v err=%v", result, err)
		}
		if stdin {
			if len(messages) != 2 || messages[0].Role != "system" || messages[0].Content != "Summarize" || messages[1].Content != document {
				t.Fatalf("messages=%#v", messages)
			}
		} else if len(messages) != 1 || messages[0].Content != "Summarize" {
			t.Fatalf("argv messages=%#v", messages)
		}
	}
	if backend.starts != 2 {
		t.Fatalf("starts=%d", backend.starts)
	}
}

func TestStdinCompletionValidationBeforeBackend(t *testing.T) {
	for _, tc := range []struct {
		name, instruction, doc string
		valid                  bool
	}{
		{"boundary", "x", strings.Repeat("a", CompletionStdinMaxBytes-1), true},
		{"over", "x", strings.Repeat("a", CompletionStdinMaxBytes), false},
		{"empty", "x", "", false}, {"blank", "x", " \n\t", false},
		{"utf8", "x", string([]byte{255}), false}, {"instruction-utf8", string([]byte{255}), "doc", false},
		{"no-instruction", " ", "doc", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := completionCommand{Prompt: tc.instruction, Stdin: true}
			_, err := completionCommandPrompt(cmd, tc.doc)
			if (err == nil) != tc.valid {
				t.Fatalf("validation err=%v", err)
			}
			if !tc.valid {
				// Nil component would reach backend acquisition if validation were bypassed.
				_, err := (*Component)(nil).handleCompletionCommand(t.Context(), commandengine.Request{Stdin: tc.doc}, cmd)
				if err == nil || strings.Contains(err.Error(), "backend error") {
					t.Fatalf("validation not enforced: %v", err)
				}
			}
		})
	}
}

func TestStdinCompletionSelectedModelAndSources(t *testing.T) {
	c := &Component{}
	for _, source := range []commandengine.Source{commandengine.SourceHostbridge, commandengine.SourceCLI, commandengine.SourceMessage} {
		router, err := commandengine.NewRouter(c.CommandDefinitions(), source)
		if err != nil {
			t.Fatal(err)
		}
		base := commandengine.Request{Context: commandengine.Context{Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}}}}
		req, err := router.Parse(t.Context(), base, []string{"model", "chosen", "completion", "Summarize", "--stdin"})
		if err != nil {
			t.Fatal(err)
		}
		cmd := req.Command.(completionCommand)
		if cmd.Model != "chosen" || !cmd.Stdin {
			t.Fatalf("cmd=%+v", cmd)
		}
		if _, err := c.handleCompletionCommand(t.Context(), req, cmd); err == nil || !strings.Contains(err.Error(), "nonblank piped document") {
			t.Fatalf("missing stdin source=%s err=%v", source, err)
		}
		if _, err := router.Parse(t.Context(), base, []string{"completion", "--stdin"}); err == nil {
			t.Fatal("missing instruction accepted")
		}
	}
}
