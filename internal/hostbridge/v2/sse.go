package v2

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

type sseStream struct {
	w       http.ResponseWriter
	flusher http.Flusher
	mu      sync.Mutex
	seq     int
}

type sseData struct {
	Seq       int    `json:"seq"`
	Text      string `json:"text,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Payload   any    `json:"payload,omitempty"`
	Summary   string `json:"summary,omitempty"`
	Error     string `json:"error,omitempty"`
	ExitCode  int    `json:"exit_code,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms,omitempty"`
}

func newSSEStream(w http.ResponseWriter) *sseStream {
	flusher, _ := w.(http.Flusher)
	return &sseStream{w: w, flusher: flusher}
}

func (s *sseStream) Stdout(line string) {
	s.write("stdout", sseData{Text: line})
}

func (s *sseStream) Stderr(line string) {
	s.write("stderr", sseData{Text: line})
}

func (s *sseStream) Event(kind string, payload any) {
	s.write("event", sseData{Kind: kind, Payload: payload})
}

func (s *sseStream) Started() {
	s.write("started", sseData{})
}

func (s *sseStream) Completed(result string, elapsed time.Duration) {
	s.write("completed", sseData{ExitCode: 0, Summary: result, ElapsedMS: elapsed.Milliseconds()})
}

func (s *sseStream) Failed(err error, elapsed time.Duration) {
	message := ""
	if err != nil {
		message = err.Error()
	}
	s.write("failed", sseData{ExitCode: 1, Error: message, ElapsedMS: elapsed.Milliseconds()})
}

func (s *sseStream) write(event string, data sseData) {
	if s == nil || s.w == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.seq++
	data.Seq = s.seq
	body, err := json.Marshal(data)
	if err != nil {
		body = []byte(`{"seq":` + fmt.Sprint(s.seq) + `,"error":"encode event"}`)
	}
	_, _ = fmt.Fprintf(s.w, "event: %s\n", event)
	_, _ = fmt.Fprintf(s.w, "data: %s\n\n", body)
	if s.flusher != nil {
		s.flusher.Flush()
	}
}

func writeSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}
