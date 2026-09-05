//go:build !darwin && !linux

package backend

import (
	"fmt"
	"os/exec"
)

func prepareNativeProcess(*exec.Cmd) error {
	return fmt.Errorf("native backend supports macOS and Linux only")
}
func signalNativeProcess(*exec.Cmd, bool) error {
	return fmt.Errorf("native backend supports macOS and Linux only")
}
