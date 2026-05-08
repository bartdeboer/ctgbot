package backend

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
)

func (s ServiceSpec) clean() ServiceSpec {
	s.BaseURL = strings.TrimSpace(s.BaseURL)
	s.HealthURL = strings.TrimSpace(s.HealthURL)
	s.Ports = cleanStrings(s.Ports)
	s.Env = cleanStrings(s.Env)
	s.Cmd = cleanStrings(s.Cmd)
	s.Mounts = cleanMounts(s.Mounts)
	return s
}

func newHealthRequest(ctx context.Context, url string) (*http.Request, error) {
	return http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSpace(url), nil)
}

func probeHealth(req *http.Request) error {
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("health status %s", resp.Status)
	}
	return nil
}

func cleanStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		out = append(out, value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func cleanMounts(values []containerengine.Mount) []containerengine.Mount {
	if len(values) == 0 {
		return nil
	}
	out := make([]containerengine.Mount, 0, len(values))
	for _, mount := range values {
		mount.Source = strings.TrimSpace(mount.Source)
		mount.Target = strings.TrimSpace(mount.Target)
		if mount.Source == "" || mount.Target == "" {
			continue
		}
		out = append(out, mount)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func safeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	prevDash := false
	for _, r := range value {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "default"
	}
	return out
}
