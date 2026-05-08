package bridge

import (
	"io"
	"log"
	"strings"
	"testing"
)

func skipIfListenerUnavailable(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	text := err.Error()
	if strings.Contains(text, "bind: operation not permitted") || (strings.Contains(text, "listen tcp") && strings.Contains(text, "operation not permitted")) {
		t.Skipf("listener unavailable in this environment: %v", err)
	}
}

func TestDefaultListenAddressMatchesEstablishedNetworkContract(t *testing.T) {
	if got, want := DefaultListenAddress, "127.0.0.1:4568"; got != want {
		t.Fatalf("DefaultListenAddress = %q, want %q", got, want)
	}
}

func TestBridgeStartIsIdempotent(t *testing.T) {
	bridge := NewBridge(t.TempDir(), nil, log.New(io.Discard, "", 0)).WithListenAddress("127.0.0.1:0")
	t.Cleanup(func() {
		_ = bridge.Close()
	})

	container1, host1, err := bridge.Start()
	if err != nil {
		skipIfListenerUnavailable(t, err)
		t.Fatalf("Start() error = %v", err)
	}
	if container1 == "" || host1 == "" {
		t.Fatalf("Start() returned empty addresses container=%q host=%q", container1, host1)
	}

	container2, host2, err := bridge.Start()
	if err != nil {
		t.Fatalf("second Start() error = %v", err)
	}
	if container2 != container1 || host2 != host1 {
		t.Fatalf("second Start() = (%q, %q), want (%q, %q)", container2, host2, container1, host1)
	}
}
