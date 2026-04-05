package codexengine

import (
	"bufio"
	"encoding/json"
	"strings"
)

type codexJSONLEvent struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
}

func extractCodexThreadID(jsonl string) string {
	scanner := bufio.NewScanner(strings.NewReader(jsonl))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev codexJSONLEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			continue
		}
		if ev.Type == "thread.started" && strings.TrimSpace(ev.ThreadID) != "" {
			return strings.TrimSpace(ev.ThreadID)
		}
	}
	return ""
}
