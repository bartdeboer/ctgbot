//go:build darwin || linux

package backend

import (
	"os/exec"
	"syscall"
)

func prepareNativeProcess(*exec.Cmd) error { return nil }

// Signal through the retained os.Process handle, never a cached numeric PID/PGID.
// A wrapper is responsible for forwarding termination to its own children.
func signalNativeProcess(cmd *exec.Cmd, force bool) error {
	if force {
		return cmd.Process.Kill()
	}
	return cmd.Process.Signal(syscall.SIGTERM)
}
