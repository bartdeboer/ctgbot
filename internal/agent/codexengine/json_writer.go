package codexengine

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
)

type codexJSONWriter struct {
	dst  io.Writer
	logf func(string, ...any)

	pending bytes.Buffer

	threadID          string
	agentMessage      string
	inputTokens       int
	cachedInputTokens int
	outputTokens      int
}

type codexJSONEvent struct {
	Type     string         `json:"type"`
	ThreadID string         `json:"thread_id"`
	Item     codexJSONItem  `json:"item"`
	Usage    codexJSONUsage `json:"usage"`
}

type codexJSONItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexJSONUsage struct {
	InputTokens       int `json:"input_tokens"`
	CachedInputTokens int `json:"cached_input_tokens"`
	OutputTokens      int `json:"output_tokens"`
}

func newCodexJSONWriter(dst io.Writer, logf func(string, ...any)) *codexJSONWriter {
	return &codexJSONWriter{dst: dst, logf: logf}
}

func (w *codexJSONWriter) Write(p []byte) (int, error) {
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

func (w *codexJSONWriter) Flush() {
	if w == nil {
		return
	}
	line := strings.TrimSpace(w.pending.String())
	w.pending.Reset()
	if line != "" {
		w.handleLine(line)
	}
}

func (w *codexJSONWriter) ThreadID() string {
	if w == nil {
		return ""
	}
	return w.threadID
}

func (w *codexJSONWriter) AgentMessage() string {
	if w == nil {
		return ""
	}
	return w.agentMessage
}

func (w *codexJSONWriter) InputTokens() int {
	if w == nil {
		return 0
	}
	return w.inputTokens
}

func (w *codexJSONWriter) CachedInputTokens() int {
	if w == nil {
		return 0
	}
	return w.cachedInputTokens
}

func (w *codexJSONWriter) OutputTokens() int {
	if w == nil {
		return 0
	}
	return w.outputTokens
}

func (w *codexJSONWriter) drainCompleteLines() {
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

func (w *codexJSONWriter) handleLine(line string) {
	w.log("codex json %s", line)

	ev, err := parseCodexJSONEvent(line)
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
		}
	case "turn.completed":
		w.inputTokens = ev.Usage.InputTokens
		w.cachedInputTokens = ev.Usage.CachedInputTokens
		w.outputTokens = ev.Usage.OutputTokens
		w.log("codex json turn completed input_tokens=%d cached_input_tokens=%d output_tokens=%d", w.inputTokens, w.cachedInputTokens, w.outputTokens)
	}
}

func (w *codexJSONWriter) log(format string, args ...any) {
	if w != nil && w.logf != nil {
		w.logf(format, args...)
	}
}

func parseCodexJSONEvent(line string) (codexJSONEvent, error) {
	var ev codexJSONEvent
	err := json.Unmarshal([]byte(strings.TrimSpace(line)), &ev)
	return ev, err
}
