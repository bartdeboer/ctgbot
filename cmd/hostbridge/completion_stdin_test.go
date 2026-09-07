package main

import (
	"bytes"
	"encoding/gob"
	"os"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component/llamacpp"
)

func TestCompletionStdinCaptureAndTypedTransport(t *testing.T) {
	t.Setenv("CTGBOT_ACTIVE_COMPONENTS", "llamacpp,llamacpp/mac-local")
	for _, prefix := range []string{"llamacpp", "llamacpp/mac-local"} {
		for _, named := range []bool{false, true} {
			args := []string{prefix}
			if named {
				args = append(args, "model", "chosen")
			}
			args = append(args, "completion", "Summarize", "--stdin")
			router, err := hostbridgeRouter(args)
			if err != nil {
				t.Fatal(err)
			}
			req, err := router.Parse(t.Context(), testHostbridgeRequest(), args)
			if err != nil {
				t.Fatal(err)
			}
			document := "  \"quotes\"\nРусский Nederlands\n\\ $HOME\n"
			pipe, writer, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = pipe.Close() })
			if _, err := writer.WriteString(document); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := captureCompletionStdin(&req, pipe); err != nil {
				t.Fatal(err)
			}
			var wire bytes.Buffer
			if err := gob.NewEncoder(&wire).Encode(req); err != nil {
				t.Fatal(err)
			}
			var decoded commandengine.Request
			if err := gob.NewDecoder(&wire).Decode(&decoded); err != nil {
				t.Fatal(err)
			}
			if decoded.Stdin != document || !llamacpp.CompletionUsesStdin(decoded.Command) {
				t.Fatalf("transport lost stdin or typed flag")
			}
			if strings.Contains(decoded.Route, document) {
				t.Fatal("document leaked into route")
			}
		}
	}
}

func TestCompletionStdinCaptureBoundsAndTerminal(t *testing.T) {
	args := []string{"llamacpp", "completion", "Summarize", "--stdin"}
	router, err := hostbridgeRouter(args)
	if err != nil {
		t.Fatal(err)
	}
	req, err := router.Parse(t.Context(), testHostbridgeRequest(), args)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []int{llamacpp.CompletionStdinMaxBytes, llamacpp.CompletionStdinMaxBytes + 1} {
		err := captureCompletionStdin(&req, &testStdin{Reader: strings.NewReader(strings.Repeat("x", n))})
		if (err != nil) != (n > llamacpp.CompletionStdinMaxBytes) {
			t.Fatalf("size=%d err=%v", n, err)
		}
	}
	reader := strings.NewReader("untouched")
	if err := captureCompletionStdin(&req, &testStdin{Reader: reader, Mode: os.ModeCharDevice}); err == nil {
		t.Fatal("terminal accepted")
	}
	if reader.Len() != len("untouched") {
		t.Fatal("read terminal")
	}
	args = []string{"llamacpp", "completion", "hello"}
	req, err = router.Parse(t.Context(), testHostbridgeRequest(), args)
	if err != nil {
		t.Fatal(err)
	}
	if err := captureCompletionStdin(&req, &testStdin{Reader: reader}); err != nil {
		t.Fatal(err)
	}
	if reader.Len() != len("untouched") {
		t.Fatal("implicit stdin read")
	}
	base := testHostbridgeRequest()
	base.Context.Actor.Roles = nil
	if _, err := router.Parse(t.Context(), base, []string{"llamacpp", "completion", "hello", "--stdin"}); err == nil {
		t.Fatal("unauthorized accepted")
	}
}
