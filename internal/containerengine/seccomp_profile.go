package containerengine

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// SeccompProfileSecurityOpts validates an operator-managed file on the Docker
// CLI host. Docker interprets the full policy; this is not a policy safety audit.
// The file and its parent directories must not be writable by agents.
func SeccompProfileSecurityOpts(path string) ([]string, error) {
	if !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
		return nil, fmt.Errorf("seccomp profile must be an absolute host file path")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("cannot stat seccomp profile: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("seccomp profile must be a regular non-symlink file")
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("cannot open seccomp profile: %w", err)
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || !os.SameFile(info, opened) {
		return nil, fmt.Errorf("seccomp profile changed during open")
	}
	const maxBytes = 1 << 20
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil || len(data) > maxBytes {
		return nil, fmt.Errorf("cannot read seccomp profile within 1 MiB limit")
	}
	var profile struct {
		DefaultAction string            `json:"defaultAction"`
		Syscalls      []json.RawMessage `json:"syscalls"`
	}
	if err := json.Unmarshal(data, &profile); err != nil || !strings.HasPrefix(profile.DefaultAction, "SCMP_ACT_") {
		return nil, fmt.Errorf("seccomp profile must be a JSON object with a defaultAction")
	}
	return []string{"seccomp=" + path}, nil
}
