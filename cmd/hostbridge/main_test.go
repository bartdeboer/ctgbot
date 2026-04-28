package main

import (
	"bytes"
	"io"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestNormalizedArgsTreatsRunSendstdinAsMediaCommand(t *testing.T) {
	got := normalizedArgs([]string{"run", "sendstdin", "-caption", "note"})
	want := []string{"sendstdin", "-caption", "note"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizedArgs() = %#v, want %#v", got, want)
	}
}

func TestPrintHelpDistinguishesStandaloneHostbridgeSurface(t *testing.T) {
	output := captureStdout(t, printHelp)
	for _, want := range []string{
		"Commands for Telegram-attached ctgbot hostbridge:",
		"Standalone ctgbot hostbridge serve accepts only:",
		"config get <key> - Show a config value",
		"model set <model> - Set the Codex model for this thread",
		"model effort set <effort> - Set the Codex reasoning effort for this thread",
		"run <command> - Run a whitelisted host command",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("help output missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "config hostbridge scaffold") {
		t.Fatalf("help output includes CLI-only scaffold command:\n%s", output)
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	prev := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = prev }()

	outC := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		outC <- buf.String()
	}()

	fn()

	_ = w.Close()
	return <-outC
}
