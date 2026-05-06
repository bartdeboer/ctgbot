package system

import (
	"context"
	"io"
	"log"
	"os"
	"strings"
	"testing"

	"github.com/bartdeboer/go-clistate"
)

func TestV5HostbridgeUsesConfiguredListenAddr(t *testing.T) {
	root := t.TempDir()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	store, err := clistate.NewCwd("ctgbot", "config")
	if err != nil {
		t.Fatalf("NewCwd: %v", err)
	}
	if err := store.PersistString("hostbridge.tcp_listen_addr", "127.0.0.1:notaport"); err != nil {
		t.Fatalf("PersistString hostbridge listen addr: %v", err)
	}

	system, err := Open(context.Background(), root, "", store, log.New(io.Discard, "", 0))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	_, _, err = system.StartHostbridge()
	if err == nil {
		t.Fatal("StartHostbridge() unexpectedly succeeded")
	}
	if !strings.Contains(err.Error(), "missing port in address") && !strings.Contains(err.Error(), "too many colons") && !strings.Contains(err.Error(), "unknown port") && !strings.Contains(err.Error(), "invalid port") {
		t.Fatalf("StartHostbridge() error = %v, want invalid address error", err)
	}
}
