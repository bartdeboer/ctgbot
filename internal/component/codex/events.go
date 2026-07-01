package codex

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

type eventWriter struct {
	dst            io.Writer
	logf           func(string, ...any)
	onAgentMessage func(string)

	pending bytes.Buffer

	threadID          string
	agentMessage      string
	inputTokens       int
	cachedInputTokens int
	outputTokens      int
}

type codexEvent struct {
	Type     string     `json:"type"`
	ThreadID string     `json:"thread_id"`
	Item     codexItem  `json:"item"`
	Usage    codexUsage `json:"usage"`
}

type codexItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexUsage struct {
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	OutputTokens      int `json:"output_tokens"`
}

func newEventWriter(dst io.Writer, logf func(string, ...any), onAgentMessage ...func(string)) *eventWriter {
	w := &eventWriter{dst: dst, logf: logf}
	if len(onAgentMessage) > 0 {
		w.onAgentMessage = onAgentMessage[0]
	}
	return w
}

func (w *eventWriter) Write(p []byte) (int, error) {
	if w == nil {
		return len(p), nil
	}
	if w.dst != nil {
		if _, err := w.dst.Write(p); err != nil {
			return 0, err
		}
	}
	w.pending.Write(p)
	w.drainCompleteLines()
	return len(p), nil
}

func (w *eventWriter) Flush() {
	if w == nil {
		return
	}
	line := strings.TrimSpace(w.pending.String())
	w.pending.Reset()
	if line != "" {
		w.handleLine(line)
	}
}

func (w *eventWriter) ThreadID() string {
	if w == nil {
		return ""
	}
	return w.threadID
}

func (w *eventWriter) AgentMessage() string {
	if w == nil {
		return ""
	}
	return w.agentMessage
}

func (w *eventWriter) InputTokens() int {
	if w == nil {
		return 0
	}
	return w.inputTokens
}

func (w *eventWriter) CachedInputTokens() int {
	if w == nil {
		return 0
	}
	return w.cachedInputTokens
}

func (w *eventWriter) OutputTokens() int {
	if w == nil {
		return 0
	}
	return w.outputTokens
}

func (w *eventWriter) drainCompleteLines() {
	for {
		data := w.pending.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			return
		}
		line := strings.TrimSpace(string(data[:idx]))
		w.pending.Next(idx + 1)
		if line != "" {
			w.handleLine(line)
		}
	}
}

func (w *eventWriter) handleLine(line string) {
	w.log("codex json %s", line)

	ev, err := parseEvent(line)
	if err != nil {
		w.log("codex json invalid line=%q", line)
		return
	}

	switch ev.Type {
	case "thread.started":
		w.threadID = strings.TrimSpace(ev.ThreadID)
		if w.threadID != "" {
			w.log("codex json thread started provider_thread_id=%s", w.threadID)
		}
	case "item.completed":
		if ev.Item.Type == "agent_message" {
			w.agentMessage = strings.TrimSpace(ev.Item.Text)
			w.log("codex json agent message chars=%d", len(w.agentMessage))
			if isCodexProtocolMessage(w.agentMessage) {
				w.log("codex json suppressed protocol agent message")
				return
			}
			if w.agentMessage != "" && w.onAgentMessage != nil {
				w.onAgentMessage(w.agentMessage)
			}
		}
	case "turn.completed":
		w.inputTokens = ev.Usage.InputTokens
		w.cachedInputTokens = ev.Usage.CachedInputTokens
		w.outputTokens = ev.Usage.OutputTokens
		w.log("codex json turn completed input_tokens=%d cached_input_tokens=%d output_tokens=%d", w.inputTokens, w.cachedInputTokens, w.outputTokens)
	}
}

func (w *eventWriter) log(format string, args ...any) {
	if w != nil && w.logf != nil {
		w.logf(format, args...)
	}
}

func isCodexProtocolMessage(text string) bool {
	if strings.Contains(text, "<tool_call>") || strings.Contains(text, "<tool_result>") {
		return true
	}
	return isCodexToolArgumentMessage(text)
}

func isCodexToolArgumentMessage(text string) bool {
	payload, ok := leadingJSONObject(strings.TrimSpace(text))
	if !ok {
		return false
	}
	for _, key := range []string{
		"cmd",        // exec_command
		"plan",       // update_plan
		"session_id", // write_stdin / shell session follow-up
	} {
		if _, ok := payload[key]; ok {
			return true
		}
	}
	return false
}

func leadingJSONObject(text string) (map[string]json.RawMessage, bool) {
	if !strings.HasPrefix(text, "{") {
		return nil, false
	}
	depth := 0
	inString := false
	escaped := false
	for i, r := range text {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch r {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch r {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				var payload map[string]json.RawMessage
				if err := json.Unmarshal([]byte(text[:i+1]), &payload); err != nil {
					return nil, false
				}
				return payload, true
			}
			if depth < 0 {
				return nil, false
			}
		}
	}
	return nil, false
}

func parseEvent(line string) (codexEvent, error) {
	var ev codexEvent
	err := json.Unmarshal([]byte(strings.TrimSpace(line)), &ev)
	return ev, err
}

func extractThreadID(jsonl string) string {
	scanner := bufio.NewScanner(strings.NewReader(jsonl))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		ev, err := parseEvent(line)
		if err != nil {
			continue
		}
		if ev.Type == "thread.started" && strings.TrimSpace(ev.ThreadID) != "" {
			return strings.TrimSpace(ev.ThreadID)
		}
	}
	return ""
}

const errorDetailMax = 4000

func trimErrorDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if len(detail) <= errorDetailMax {
		return detail
	}
	return strings.TrimSpace(detail[:errorDetailMax]) + "..."
}
