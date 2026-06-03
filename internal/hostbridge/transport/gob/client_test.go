package gobtransport

import (
	"bytes"
	"context"
	"encoding/gob"
	"errors"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/hostbridge"
)

type fakeByteTransport struct {
	payload []byte
	resp    hostbridge.CommandResponse
	err     error
}

func (t *fakeByteTransport) Send(_ context.Context, payload []byte) ([]byte, error) {
	t.payload = append([]byte(nil), payload...)
	if t.err != nil {
		return nil, t.err
	}
	var out bytes.Buffer
	if err := gob.NewEncoder(&out).Encode(t.resp); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

func TestCommandRunnerEncodesRequestAndDecodesResponse(t *testing.T) {
	transport := &fakeByteTransport{resp: hostbridge.CommandResponse{Result: commandengine.Result{Text: "ok"}}}
	runner := &CommandRunner{Transport: transport}
	resp, err := runner.RunCommand(context.Background(), hostbridge.CommandRequest{
		Request: commandengine.Request{Stdin: "hello"},
	})
	if err != nil {
		t.Fatalf("RunCommand() error = %v", err)
	}
	if resp.Result.Text != "ok" {
		t.Fatalf("response text = %q, want ok", resp.Result.Text)
	}
	var decoded hostbridge.CommandRequest
	if err := gob.NewDecoder(bytes.NewReader(transport.payload)).Decode(&decoded); err != nil {
		t.Fatalf("decode sent payload: %v", err)
	}
	if decoded.Request.Stdin != "hello" {
		t.Fatalf("sent stdin = %q, want hello", decoded.Request.Stdin)
	}
}

func TestCommandRunnerReturnsCommandError(t *testing.T) {
	runner := &CommandRunner{Transport: &fakeByteTransport{resp: hostbridge.CommandResponse{Error: "boom"}}}
	resp, err := runner.RunCommand(context.Background(), hostbridge.CommandRequest{})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("RunCommand() error = %v, want boom", err)
	}
	if resp.Error != "boom" {
		t.Fatalf("response error = %q, want boom", resp.Error)
	}
}

func TestCommandRunnerReturnsTransportError(t *testing.T) {
	want := errors.New("transport down")
	runner := &CommandRunner{Transport: &fakeByteTransport{err: want}}
	_, err := runner.RunCommand(context.Background(), hostbridge.CommandRequest{})
	if !errors.Is(err, want) {
		t.Fatalf("RunCommand() error = %v, want %v", err, want)
	}
}
