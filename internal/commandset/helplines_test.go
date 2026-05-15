package commandset

import (
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
	"github.com/bartdeboer/go-clir"
)

// threadHelpDefinitions mirrors the messaging component's thread definitions.
func threadHelpDefinitions() []commandengine.Definition {
	return []commandengine.Definition{
		{
			Pattern: "status",
			Help:    "Show current thread status",
			Build:   helpTestBuild,
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent, simplerbac.RoleUser),
			Aliases: []commandengine.Route{
				{Pattern: "thread status", Absolute: true},
				{Pattern: "thread current status", Absolute: true},
			},
		},
		{
			Pattern: "thread list",
			Help:    "List recent active threads",
			Build:   helpTestBuild,
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		},
		{
			Pattern: "thread <thread> status",
			Help:    "Show referenced thread status",
			Build:   helpTestBuild,
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		},
		{
			Pattern: "thread <thread> message send",
			Help:    "Send a message into another thread",
			Build:   helpTestBuild,
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		},
		{
			Pattern: "thread <thread> component bind <component>",
			Help:    "Bind a component provider thread to a thread",
			Build:   helpTestBuild,
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
	}
}

func helpTestBuild(req *clir.Request) (any, error) { _ = req; return testCommand{}, nil }

func TestHelpLinesRoleAgentHidesRootOnlyCommands(t *testing.T) {
	definitions := []commandengine.Definition{
		{
			Pattern: "root-only command",
			Help:    "Root only",
			Build:   helpTestBuild,
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
		{
			Pattern: "agent command",
			Help:    "Agent allowed",
			Build:   helpTestBuild,
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleAgent),
		},
		{
			Pattern: "public command",
			Help:    "Anyone",
			Build:   helpTestBuild,
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  simplerbac.Public(),
		},
	}

	lines := HelpLines(definitions, HelpOptions{
		Actor: coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
	})

	if !helpLinesContain(lines, "agent command") {
		t.Fatalf("HelpLines() missing agent command in %v", lines)
	}
	if !helpLinesContain(lines, "public command") {
		t.Fatalf("HelpLines() missing public command in %v", lines)
	}
	if helpLinesContain(lines, "root-only command") {
		t.Fatalf("HelpLines() unexpectedly contains root-only command in %v", lines)
	}
}

func TestHelpLinesRoleRootSeesRootOnlyCommands(t *testing.T) {
	definitions := []commandengine.Definition{
		{
			Pattern: "root-only command",
			Help:    "Root only",
			Build:   helpTestBuild,
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot),
		},
		{
			Pattern: "shared command",
			Help:    "Root and agent",
			Build:   helpTestBuild,
			Sources: []commandengine.Source{commandengine.SourceHostbridge},
			Policy:  simplerbac.Any(simplerbac.RoleRoot, simplerbac.RoleAgent),
		},
	}

	agentLines := HelpLines(definitions, HelpOptions{
		Actor: coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
	})
	rootLines := HelpLines(definitions, HelpOptions{
		Actor: coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}},
	})

	if helpLinesContain(agentLines, "root-only command") {
		t.Fatalf("HelpLines() agent unexpectedly sees root-only command in %v", agentLines)
	}
	if !helpLinesContain(rootLines, "root-only command") {
		t.Fatalf("HelpLines() missing root-only command for root actor in %v", rootLines)
	}
	if !helpLinesContain(rootLines, "shared command") {
		t.Fatalf("HelpLines() missing shared command for root actor in %v", rootLines)
	}
}

func TestHelpLinesThreadHelpIncludesMessageSend(t *testing.T) {
	lines := HelpLines(threadHelpDefinitions(), HelpOptions{
		Source: commandengine.SourceHostbridge,
		Actor:  coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		Scope:  []string{"thread"},
	})

	if !helpLinesContain(lines, "thread <thread> message send") {
		t.Fatalf("HelpLines() missing thread message send in %v", lines)
	}
}

func TestHelpLinesThreadHelpHidesRootOnlyComponentBind(t *testing.T) {
	lines := HelpLines(threadHelpDefinitions(), HelpOptions{
		Source: commandengine.SourceHostbridge,
		Actor:  coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		Scope:  []string{"thread"},
	})

	if helpLinesContain(lines, "thread <thread> component bind") {
		t.Fatalf("HelpLines() unexpectedly contains root-only bind command for agent in %v", lines)
	}

	rootLines := HelpLines(threadHelpDefinitions(), HelpOptions{
		Source: commandengine.SourceHostbridge,
		Actor:  coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleRoot}},
		Scope:  []string{"thread"},
	})
	if !helpLinesContain(rootLines, "thread <thread> component bind") {
		t.Fatalf("HelpLines() missing bind command for root actor in %v", rootLines)
	}
}

func TestHelpLinesNoDuplicateLines(t *testing.T) {
	// status definition has thread status and thread current status as aliases;
	// only the first matching route per definition should appear.
	lines := HelpLines(threadHelpDefinitions(), HelpOptions{
		Source: commandengine.SourceHostbridge,
		Actor:  coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		Scope:  []string{"thread"},
	})

	seen := map[string]int{}
	for _, line := range lines {
		seen[line]++
	}
	for line, count := range seen {
		if count > 1 {
			t.Fatalf("HelpLines() duplicate line %q appears %d times in %v", line, count, lines)
		}
	}
}

func TestHelpLinesScopedStyleIsNoIndent(t *testing.T) {
	lines := HelpLines(threadHelpDefinitions(), HelpOptions{
		Source: commandengine.SourceHostbridge,
		Actor:  coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		Scope:  []string{"thread"},
	})

	for _, line := range lines {
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			t.Fatalf("HelpLines() line has leading whitespace: %q", line)
		}
	}
}

func TestHelpLinesCurrentScopeUsesLiteralMatching(t *testing.T) {
	// scope ["thread", "current"] should show thread current status
	// but NOT thread <thread> status or other <thread> variants.
	lines := HelpLines(threadHelpDefinitions(), HelpOptions{
		Source: commandengine.SourceHostbridge,
		Actor:  coremodel.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		Scope:  []string{"thread", "current"},
	})

	if !helpLinesContain(lines, "thread current status") {
		t.Fatalf("HelpLines() missing thread current status in %v", lines)
	}
	for _, line := range lines {
		if strings.Contains(line, "thread <thread>") {
			t.Fatalf("HelpLines() unexpectedly contains parameterized route %q for literal scope", line)
		}
	}
}

func helpLinesContain(lines []string, substr string) bool {
	for _, line := range lines {
		if strings.Contains(line, substr) {
			return true
		}
	}
	return false
}
