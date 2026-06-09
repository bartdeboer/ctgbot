package commandengine

import (
	"strings"
	"testing"
)

func TestCommandSynopsisRendersCommandTrie(t *testing.T) {
	got := CommandSynopsis(
		"hostbridge",
		[]string{
			"hostbridge codex chat purge",
			"hostbridge codex compact",
			"hostbridge codex config get <key>",
			"hostbridge codex config list",
			"hostbridge codex config set <key> <value>",
			"hostbridge theater list",
			"hostbridge theater status",
			"hostbridge theater <thread> read",
			"hostbridge theater <thread> subscribe",
		},
		map[string]string{
			"theater": "thread subscriptions and shared message boards",
		},
	)
	for _, want := range []string{
		"hostbridge [",
		"codex [",
		"chat purge",
		"config [ get <key> | list | set <key> <value> ]",
		"theater [ # thread subscriptions and shared message boards",
		"<thread> [ read | subscribe ]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("CommandSynopsis() missing %q:\n%s", want, got)
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
