package copilot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

type TurnResult struct {
	Reply            string
	ProviderThreadID string
}

const (
	maxEventBytes  = 1024 * 1024
	maxOutputBytes = 32 * 1024 * 1024
	maxReplyBytes  = 1024 * 1024
)

// eventWriter consumes the pinned CLI's JSONL, NOT its entire session log schema.
// Unknown event types are ignored, but malformed framing fails closed. On error
// Write keeps draining so parser rejection cannot strand the provider on stdout.
type eventWriter struct {
	expected              string
	pending               []byte
	total                 int
	err                   error
	terminal              bool
	exitCode              int
	reply                 string
	chunkNext, chunkCount int
	chunkAPI              string
	chunks                strings.Builder
}

func newEventWriter(expected string) *eventWriter { return &eventWriter{expected: expected} }

func (w *eventWriter) Write(p []byte) (int, error) {
	size := len(p)
	if w.err != nil {
		return size, nil
	}
	if size > maxOutputBytes-w.total {
		w.err = fmt.Errorf("copilot output exceeds limit")
		return size, nil
	}
	w.total += size
	for len(p) > 0 && w.err == nil {
		end := bytes.IndexByte(p, '\n')
		part := p
		if end >= 0 {
			part = p[:end]
		}
		if len(w.pending)+len(part) > maxEventBytes {
			w.err = fmt.Errorf("copilot JSONL record exceeds limit")
			break
		}
		w.pending = append(w.pending, part...)
		if end < 0 {
			break
		}
		w.consume(w.pending)
		w.pending = w.pending[:0]
		p = p[end+1:]
	}
	return size, nil
}

type outputEvent struct {
	Type      string          `json:"type"`
	AgentID   string          `json:"agentId"`
	SessionID string          `json:"sessionId"`
	ExitCode  *int            `json:"exitCode"`
	Data      json.RawMessage `json:"data"`
}

type assistantMessage struct {
	MessageID        string            `json:"messageId"`
	Content          *string           `json:"content"`
	ParentToolCallID string            `json:"parentToolCallId"`
	ToolRequests     []json.RawMessage `json:"toolRequests"`
	Phase            string            `json:"phase"`
	ChunkIndex       *int              `json:"chunkIndex"`
	ChunkCount       *int              `json:"chunkCount"`
	APIID            string            `json:"apiCallId"`
}

func (w *eventWriter) consume(line []byte) {
	if len(bytes.TrimSpace(line)) == 0 {
		return
	}
	var event outputEvent
	if err := json.Unmarshal(line, &event); err != nil || event.Type == "" {
		w.err = fmt.Errorf("invalid copilot JSONL event")
		return
	}
	if w.terminal {
		w.err = fmt.Errorf("copilot emitted events after terminal result")
		return
	}
	if event.AgentID != "" {
		return
	}
	switch event.Type {
	case "result":
		if err := w.checkID(event.SessionID); err != nil {
			w.err = err
			return
		}
		if event.ExitCode == nil || (*event.ExitCode != 0 && *event.ExitCode != 1) {
			w.err = fmt.Errorf("invalid copilot terminal exit code")
			return
		}
		w.terminal, w.exitCode = true, *event.ExitCode
	case "session.start":
		var data struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			w.err = fmt.Errorf("invalid copilot start event")
			return
		}
		w.err = w.checkID(data.SessionID)
	case "assistant.message":
		var message assistantMessage
		if err := json.Unmarshal(event.Data, &message); err != nil || message.MessageID == "" || message.Content == nil {
			w.err = fmt.Errorf("invalid copilot assistant message")
			return
		}
		if message.ParentToolCallID != "" {
			return
		}
		w.acceptMessage(message)
		// Deltas/reasoning/tool events are never a fallback answer. A final full
		// assistant.message plus result is required even if the CLI emits deltas.
	}
}

func (w *eventWriter) checkID(id string) error {
	canonical, err := canonicalSessionID(id)
	if err != nil || canonical != w.expected {
		return fmt.Errorf("copilot session UUID mismatch or invalid identity")
	}
	return nil
}

func (w *eventWriter) acceptMessage(m assistantMessage) {
	if len(m.ToolRequests) > 0 || m.Phase == "thinking" || m.Phase == "commentary" {
		w.reply = ""
		w.chunkCount, w.chunkNext = 0, 0
		w.chunks.Reset()
		return
	}
	content := *m.Content
	if len(content) > maxReplyBytes {
		w.err = fmt.Errorf("copilot reply exceeds limit")
		return
	}
	if m.ChunkIndex == nil && m.ChunkCount == nil {
		w.reply = content // The last full main-agent answer, not intermediate transcript.
		w.chunkCount, w.chunkNext = 0, 0
		w.chunks.Reset()
		return
	}
	if m.ChunkIndex == nil || m.ChunkCount == nil || *m.ChunkCount < 1 || *m.ChunkCount > 1024 || *m.ChunkIndex < 0 || *m.ChunkIndex >= *m.ChunkCount {
		w.err = fmt.Errorf("invalid copilot message chunks")
		return
	}
	if *m.ChunkIndex == 0 {
		w.reply = ""
		w.chunkNext, w.chunkCount, w.chunkAPI = 0, *m.ChunkCount, m.APIID
		w.chunks.Reset()
	}
	if *m.ChunkIndex != w.chunkNext || *m.ChunkCount != w.chunkCount || m.APIID != w.chunkAPI || w.chunks.Len()+len(content) > maxReplyBytes {
		w.err = fmt.Errorf("incomplete or oversized copilot message chunks")
		return
	}
	w.chunks.WriteString(content)
	w.chunkNext++
	if w.chunkNext == w.chunkCount {
		w.reply = w.chunks.String()
	}
}

func (w *eventWriter) finish() (TurnResult, error) {
	if w.err == nil && len(w.pending) > 0 {
		w.consume(w.pending)
		w.pending = nil
	}
	if w.err != nil {
		return TurnResult{}, w.err
	}
	if !w.terminal {
		return TurnResult{}, fmt.Errorf("copilot returned no terminal result")
	}
	if w.exitCode != 0 {
		// Deliberate policy: a well-formed failed terminal result confirms the expected
		// session and permits resuming that failed turn. Never infer identity from a
		// partial/start-only stream, stderr or malformed/mismatched output.
		return TurnResult{ProviderThreadID: w.expected}, fmt.Errorf("copilot reported a failed turn")
	}
	if strings.TrimSpace(w.reply) == "" || w.chunkNext != w.chunkCount {
		return TurnResult{}, fmt.Errorf("copilot returned an empty or incomplete reply")
	}
	return TurnResult{Reply: strings.TrimSpace(w.reply), ProviderThreadID: w.expected}, nil
}
