package main

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

func TestNormalizedAliasHelpPassesThroughTypedParser(t *testing.T) {
	t.Setenv("CTGBOT_ACTIVE_COMPONENTS", "codex")
	router, err := hostbridgeRouter()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		argv  []string
		alias string
		extra []string
	}{
		{[]string{"run", "docker", "--help"}, "docker", []string{"--help"}},
		{[]string{"run", "docker", "container", "help"}, "docker", []string{"container", "help"}},
		{[]string{"run", "docker", "—help"}, "docker", []string{"—help"}},
		{[]string{"run", "docker", "--label", "help"}, "docker", []string{"--label", "help"}},
		{[]string{"run", "docker", "--label", "help", "--", ""}, "docker", []string{"--label", "help", "--", ""}},
		{[]string{"docker", "--help"}, "docker", []string{"--help"}},
		{[]string{"not-an-allowed-alias", "--help"}, "not-an-allowed-alias", []string{"--help"}},
	} {
		t.Run(tc.alias+":"+strings.Join(tc.argv, " "), func(t *testing.T) {
			args := normalizedArgs(tc.argv, "codex")
			var out bytes.Buffer
			req, handled, err := parseOrRenderHelp(context.Background(), router, testHostbridgeRequest(), args, &out)
			if err != nil || handled || out.Len() != 0 {
				t.Fatalf("handled=%v err=%v output=%s", handled, err, out.String())
			}
			cmd, ok := req.Command.(schemacommands.RunCommand)
			if !ok || cmd.Command != tc.alias || !reflect.DeepEqual(cmd.Args, tc.extra) || req.CanonicalPattern != "run <command>" {
				t.Fatalf("unexpected typed request: %#v", req)
			}
		})
	}
}

func TestNormalizedWrapperAndComponentHelpStillLocal(t *testing.T) {
	t.Setenv("CTGBOT_ACTIVE_COMPONENTS", "codex")
	router, err := hostbridgeRouter()
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"help"}, {"run", "help"}, {"run", "--help"}, {"run", "—help"}, {"codex", "--help"}} {
		var out bytes.Buffer
		_, handled, err := parseOrRenderHelp(context.Background(), router, testHostbridgeRequest(), normalizedArgs(args, "codex"), &out)
		if err != nil || !handled || out.Len() == 0 {
			t.Fatalf("%q handled=%v err=%v output=%s", args, handled, err, out.String())
		}
	}
}

func TestNormalizedExactHelpRoutesRemainExecutable(t *testing.T) {
	t.Setenv("CTGBOT_ACTIVE_COMPONENTS", "codex")
	router, err := hostbridgeRouter()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		argv    []string
		pattern string
	}{
		{[]string{"component", "gmail/work", "help"}, "component <component> help"},
		{[]string{"codex", "config", "set", "model", "help"}, "codex config set <key> <value>"},
	} {
		var out bytes.Buffer
		req, handled, err := parseOrRenderHelp(context.Background(), router, testHostbridgeRequest(), normalizedArgs(tc.argv, "codex"), &out)
		if err != nil || handled || out.Len() != 0 || req.CanonicalPattern != tc.pattern {
			t.Fatalf("%q: handled=%v err=%v pattern=%q output=%s", tc.argv, handled, err, req.CanonicalPattern, out.String())
		}
	}
}
