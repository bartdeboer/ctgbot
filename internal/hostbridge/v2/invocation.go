package v2

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
)

type Invocation struct {
	Command []string
	Flags   url.Values
	Stdin   string
	Help    bool
}

func (inv Invocation) Argv() []string {
	argv := append([]string(nil), inv.Command...)
	return append(argv, flagsFromValues(inv.Flags)...)
}

func EncodeCommandPath(argv []string) (string, error) {
	segments := make([]string, 0, len(argv))
	for _, part := range argv {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		segments = append(segments, url.PathEscape(part))
	}
	if len(segments) == 0 {
		return "", fmt.Errorf("missing command")
	}
	return strings.Join(segments, "/"), nil
}

func DecodeCommandPath(path string) ([]string, error) {
	path = strings.Trim(path, "/")
	if path == "" {
		return nil, fmt.Errorf("missing command")
	}
	var argv []string
	for _, raw := range strings.Split(path, "/") {
		if raw == "" {
			continue
		}
		part, err := url.PathUnescape(raw)
		if err != nil {
			return nil, fmt.Errorf("decode path segment: %w", err)
		}
		if strings.TrimSpace(part) != "" {
			argv = append(argv, part)
		}
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("missing command")
	}
	return argv, nil
}

func DecodeInvocation(req *http.Request) (Invocation, error) {
	if req == nil || req.URL == nil {
		return Invocation{}, fmt.Errorf("missing request URL")
	}
	path := strings.TrimPrefix(req.URL.EscapedPath(), defaultRunPrefix)
	if path == req.URL.EscapedPath() {
		return Invocation{}, fmt.Errorf("expected path %s<command>", defaultRunPrefix)
	}
	command, err := DecodeCommandPath(path)
	if err != nil {
		return Invocation{}, err
	}
	var stdin string
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return Invocation{}, fmt.Errorf("read request body: %w", err)
		}
		stdin = string(body)
	}
	invocation := Invocation{
		Command: command,
		Flags:   cloneValues(req.URL.Query()),
		Stdin:   stdin,
	}
	if help, ok := commandengine.ParseHelpRequest(invocation.Argv()); ok {
		invocation.Help = true
		invocation.Command = append([]string(nil), help.Scope...)
		invocation.Flags = nil
	}
	return invocation, nil
}

func flagsFromValues(values url.Values) []string {
	if len(values) == 0 {
		return nil
	}
	var flags []string
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		items := values[key]
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		flag := "--" + key
		if len(items) == 0 {
			flags = append(flags, flag)
			continue
		}
		for _, value := range items {
			value = strings.TrimSpace(value)
			switch strings.ToLower(value) {
			case "":
				flags = append(flags, flag)
			case "true":
				flags = append(flags, flag)
			case "false":
				continue
			default:
				flags = append(flags, flag, value)
			}
		}
	}
	return flags
}

func cloneValues(values url.Values) url.Values {
	if values == nil {
		return nil
	}
	out := make(url.Values, len(values))
	for key, items := range values {
		out[key] = append([]string(nil), items...)
	}
	return out
}
