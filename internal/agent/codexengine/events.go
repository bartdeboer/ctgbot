package codexengine

import (
	"bufio"
	"strings"
)

func extractCodexThreadID(jsonl string) string {
	scanner := bufio.NewScanner(strings.NewReader(jsonl))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		ev, err := parseCodexJSONEvent(line)
		if err != nil {
			continue
		}
		if ev.Type == "thread.started" && strings.TrimSpace(ev.ThreadID) != "" {
			return strings.TrimSpace(ev.ThreadID)
		}
	}
	return ""
}
