package commandengine

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

type testCommand struct {
	Value string
}

func TestRouterFiltersBySourceAndChecksPolicy(t *testing.T) {
	definitions := []Definition{
		{
			Pattern: "only-cli <value>",
			Build: func(req *clir.Request) (any, error) {
				return testCommand{Value: req.Params["value"]}, nil
			},
			Sources: []Source{SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
		{
			Pattern: "only-message <value>",
			Build: func(req *clir.Request) (any, error) {
				return testCommand{Value: req.Params["value"]}, nil
			},
			Sources: []Source{SourceMessage},
			Policy:  simplerbac.Any(simplerbac.RoleUser),
		},
	}

	router, err := NewRouter(definitions, SourceMessage)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	_, err = router.Parse(context.Background(), Request{
		Context: Context{Actor: Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}}},
	}, []string{"only-message", "x"})
	if err == nil || !strings.Contains(err.Error(), "command only-message <value> denied") {
		t.Fatalf("Parse() error = %v, want policy denial", err)
	}

	req, err := router.Parse(context.Background(), Request{
		Context: Context{Actor: Actor{Roles: []simplerbac.Role{simplerbac.RoleUser}}},
	}, []string{"only-message", "x"})
	if err != nil {
		t.Fatalf("Parse() as user error = %v", err)
	}
	if req.Context.Source != SourceMessage {
		t.Fatalf("source = %q, want message", req.Context.Source)
	}
	if cmd, ok := req.Command.(testCommand); !ok || cmd.Value != "x" {
		t.Fatalf("command = %#v, want testCommand x", req.Command)
	}

	_, err = router.Parse(context.Background(), Request{
		Context: Context{Actor: Actor{Roles: []simplerbac.Role{simplerbac.RoleUser}}},
	}, []string{"only-cli", "x"})
	if err == nil {
		t.Fatal("Parse() for filtered route succeeded, want error")
	}
}

func TestRouterRejectsDuplicateRoutes(t *testing.T) {
	_, err := NewRouter([]Definition{
		testDefinition("one", "one", Route{Pattern: "same"}),
		testDefinition("two", "two", Route{Pattern: "same"}),
	}, SourceCLI)
	if err == nil || !strings.Contains(err.Error(), `duplicate command route "same"`) {
		t.Fatalf("NewRouter() error = %v, want duplicate route", err)
	}
}

func TestRouterHelpHidesHiddenRoutes(t *testing.T) {
	router, err := NewRouter([]Definition{
		testDefinition("visible help", "visible"),
		testDefinition("target help", "target", Route{Pattern: "legacy", Hidden: true}),
		{
			Pattern: "hidden-canonical",
			Help:    "hidden help",
			Hidden:  true,
			Build:   func(req *clir.Request) (any, error) { return testCommand{}, nil },
			Sources: []Source{SourceCLI},
			Policy:  simplerbac.Public(),
		},
	}, SourceCLI)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	var buf bytes.Buffer
	if err := router.FPrintHelp(context.Background(), &buf, nil); err != nil {
		t.Fatalf("FPrintHelp() error = %v", err)
	}

	out := buf.String()
	for _, want := range []string{"visible", "target"} {
		if !strings.Contains(out, want) {
			t.Fatalf("FPrintHelp() missing %q in %q", want, out)
		}
	}
	for _, notWant := range []string{"legacy", "hidden-canonical"} {
		if strings.Contains(out, notWant) {
			t.Fatalf("FPrintHelp() unexpectedly contains hidden route %q in %q", notWant, out)
		}
	}
}

func TestRouterMatchUsesResolveWithoutBuildingCommand(t *testing.T) {
	buildCalls := 0
	router, err := NewRouter([]Definition{{
		Pattern: "codex model",
		Help:    "Show model",
		Build: func(req *clir.Request) (any, error) {
			buildCalls++
			return testCommand{}, nil
		},
		Sources: []Source{SourceCLI},
		Policy:  simplerbac.Public(),
	}}, SourceCLI)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	match, err := router.Match(context.Background(), []string{"codex", "model", "help"})
	if err != nil {
		t.Fatalf("Match() error = %v", err)
	}
	if !match.Matched || !match.Executable || match.Exact {
		t.Fatalf("Match() = %#v, want matched executable prefix", match)
	}
	if buildCalls != 0 {
		t.Fatalf("Match() called builder %d times, want 0", buildCalls)
	}

	match, err = router.Match(context.Background(), []string{"codex", "model"})
	if err != nil {
		t.Fatalf("Match() exact error = %v", err)
	}
	if !match.Matched || !match.Executable || !match.Exact {
		t.Fatalf("Match() exact = %#v, want exact executable match", match)
	}
}

func TestParseHelpRequestUsesClirConvention(t *testing.T) {
	req, ok := ParseHelpRequest([]string{"codex", "model", "--help"})
	if !ok {
		t.Fatal("ParseHelpRequest() ok = false, want true")
	}
	if got, want := strings.Join(req.Scope, " "), "codex model"; got != want {
		t.Fatalf("Scope = %q, want %q", got, want)
	}

	if _, ok := ParseHelpRequest([]string{"codex", "model"}); ok {
		t.Fatal("ParseHelpRequest(non-help) ok = true, want false")
	}
}

func testDefinition(help string, pattern string, aliases ...Route) Definition {
	return Definition{
		Pattern: pattern,
		Help:    help,
		Build:   func(req *clir.Request) (any, error) { return testCommand{}, nil },
		Sources: []Source{SourceCLI},
		Policy:  simplerbac.Public(),
		Aliases: aliases,
	}
}

func TestRouterFPrintHelpFiltersByActorPolicy(t *testing.T) {
	router, err := NewRouter([]Definition{
		{
			Pattern: "root secret",
			Help:    "Root only",
			Build:   func(req *clir.Request) (any, error) { return testCommand{}, nil },
			Sources: []Source{SourceMessage},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
		{
			Pattern: "user status",
			Help:    "User allowed",
			Build:   func(req *clir.Request) (any, error) { return testCommand{}, nil },
			Sources: []Source{SourceMessage},
			Policy:  simplerbac.Any(simplerbac.RoleUser),
		},
	}, SourceMessage)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	var buf bytes.Buffer
	actor := Actor{Roles: []simplerbac.Role{simplerbac.RoleUser}}
	if err := router.FPrintHelp(context.Background(), &buf, nil, actor); err != nil {
		t.Fatalf("FPrintHelp() error = %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "root secret") {
		t.Fatalf("FPrintHelp() exposed root-only command: %q", out)
	}
	if !strings.Contains(out, "user status - User allowed") {
		t.Fatalf("FPrintHelp() missing user command: %q", out)
	}
}
