package toolloop

import (
	"encoding/json"
	"io"
	"sync"
	"time"
)

// Event is one chronological JSONL record from a toolloop run.
//
// The schema is intentionally small and stable: Type identifies the event,
// Iteration/ToolCall/ToolName provide common indexes, Error carries failures,
// and Data carries event-specific details.
type Event struct {
	Type      string         `json:"type"`
	Time      time.Time      `json:"time"`
	Iteration int            `json:"iteration,omitempty"`
	ToolCall  string         `json:"tool_call,omitempty"`
	ToolName  string         `json:"tool_name,omitempty"`
	Error     string         `json:"error,omitempty"`
	Data      map[string]any `json:"data,omitempty"`
}

type EventSink interface {
	Emit(Event)
}

type JSONLEventSink struct {
	mu sync.Mutex
	w  io.Writer
}

func NewJSONLEventSink(w io.Writer) *JSONLEventSink {
	if w == nil {
		return nil
	}
	return &JSONLEventSink{w: w}
}

func (s *JSONLEventSink) Emit(event Event) {
	if s == nil || s.w == nil {
		return
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	data, err := json.Marshal(event)
	if err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	_, _ = s.w.Write(append(data, '\n'))
}

func (r Runner) emit(event Event) {
	if r.Events != nil {
		r.Events.Emit(event)
	}
}
