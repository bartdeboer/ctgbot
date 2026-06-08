package agentcommon

import (
	"strings"
	"testing"
)

func TestHostbridgeSynopsisRendersCommandTrie(t *testing.T) {
	got := HostbridgeSynopsis(
		[]string{
			"hostbridge codex chat purge",
			"hostbridge codex compact",
			"hostbridge codex config get <key>",
			"hostbridge codex config list",
			"hostbridge codex config set <key> <value>",
			"hostbridge theater create <name>",
			"hostbridge theater <name> post <message>",
			"hostbridge theater <name> read",
			"hostbridge theater <name> subscribe",
		},
		map[string]string{
			"theater": "publish/subscribe message channels between agents",
		},
	)
	for _, want := range []string{
		"hostbridge [",
		"codex [",
		"chat purge",
		"config [ get <key> | list | set <key> <value> ]",
		"theater [ # publish/subscribe message channels between agents",
		"<name> [ post <message> | read | subscribe ]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("HostbridgeSynopsis() missing %q:\n%s", want, got)
		}
	}
}

func TestCommandSynopsisExpandsAliasSubcommands(t *testing.T) {
	got := CommandSynopsis("hostbridge", []string{
		"docker",
		"git-ctgbot [ fetch | pull | push | status ]",
	})
	for _, want := range []string{
		"docker",
		"git-ctgbot [ fetch | pull | push | status ]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("CommandSynopsis() missing %q:\n%s", want, got)
		}
	}
}
