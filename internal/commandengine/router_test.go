package commandengine

import (
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
			ID:      "test.cli",
			Sources: []Source{SourceCLI},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
			Routes: []Route{{
				Pattern: "only-cli <value>",
				Build: func(req *clir.Request) (any, error) {
					return testCommand{Value: req.Params["value"]}, nil
				},
			}},
		},
		{
			ID:      "test.message",
			Sources: []Source{SourceMessage},
			Policy:  simplerbac.Any(simplerbac.RoleUser),
			Routes: []Route{{
				Pattern: "only-message <value>",
				Build: func(req *clir.Request) (any, error) {
					return testCommand{Value: req.Params["value"]}, nil
				},
			}},
		},
	}

	router, err := NewRouter(definitions, SourceMessage)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	_, err = router.Parse(context.Background(), Request{
		Context: Context{Actor: Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}}},
	}, []string{"only-message", "x"})
	if err == nil || !strings.Contains(err.Error(), "command test.message denied") {
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
		testDefinition("one", "same"),
		testDefinition("two", "same"),
	}, SourceCLI)
	if err == nil || !strings.Contains(err.Error(), `duplicate command route "same"`) {
		t.Fatalf("NewRouter() error = %v, want duplicate route", err)
	}
}

func testDefinition(id string, pattern string) Definition {
	return Definition{
		ID:      id,
		Sources: []Source{SourceCLI},
		Policy:  simplerbac.Public(),
		Routes: []Route{{
			Pattern: pattern,
			Build:   func(req *clir.Request) (any, error) { return testCommand{}, nil },
		}},
	}
}
