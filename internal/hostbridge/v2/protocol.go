package v2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

const (
	// defaultRunPrefix is intentionally POST-only for the first v2 slice:
	// one HTTP endpoint routes command-shaped URLs into the command engine.
	// If v2 later grows HTTP verb semantics, those routes can live alongside
	// or replace this /run/ prefix without changing the command envelope.
	defaultRunPrefix = "/v2/run/"
)

type Handler struct {
	Runner commandengine.CommandRunner
	Source commandengine.Source
	Auth   Authenticator
}

type JSONResponse struct {
	CommandID string `json:"command_id,omitempty"`
	ExitCode  int    `json:"exit_code"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	Error     string `json:"error,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

func NewHandler(runner commandengine.CommandRunner) *Handler {
	return &Handler{Runner: runner}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if h == nil || h.Runner == nil {
		http.Error(w, "hostbridgev2 command runner unavailable", http.StatusServiceUnavailable)
		return
	}
	invocation, err := DecodeInvocation(req)
	if err != nil {
		writeError(w, req, http.StatusBadRequest, err, 0)
		return
	}
	base, err := h.baseRequestFromRequest(req)
	if err != nil {
		writeError(w, req, http.StatusUnauthorized, err, 0)
		return
	}
	base.Stdin = invocation.Stdin
	argv := invocation.Argv()
	started := time.Now()
	if invocation.Help {
		result, err := h.help(req.Context(), base, invocation.Command)
		elapsed := time.Since(started).Milliseconds()
		if err != nil {
			writeError(w, req, http.StatusOK, err, elapsed)
			return
		}
		writeResult(w, req, result.Text, elapsed)
		return
	}
	if wantsSSE(req) {
		writeSSEHeaders(w)
		stream := newSSEStream(w)
		base.OutputStream = stream
		stream.Started()
		result, err := h.Runner.Run(req.Context(), base, argv)
		elapsed := time.Since(started)
		if err != nil {
			stream.Failed(err, elapsed)
			return
		}
		stream.Completed(result.Text, elapsed)
		return
	}
	result, err := h.Runner.Run(req.Context(), base, argv)
	elapsed := time.Since(started).Milliseconds()
	if err != nil {
		writeError(w, req, http.StatusOK, err, elapsed)
		return
	}
	writeResult(w, req, result.Text, elapsed)
}

func (h *Handler) help(ctx context.Context, base commandengine.Request, scope []string) (commandengine.Result, error) {
	if h == nil || h.Runner == nil {
		return commandengine.Result{}, fmt.Errorf("hostbridgev2 command runner unavailable")
	}
	helper, ok := h.Runner.(commandengine.CommandHelper)
	if !ok || helper == nil {
		return commandengine.Result{}, fmt.Errorf("hostbridgev2 command helper unavailable")
	}
	return helper.Help(ctx, base, scope)
}

func (h *Handler) baseRequestFromRequest(req *http.Request) (commandengine.Request, error) {
	source := commandengine.SourceHostbridge
	if h != nil && h.Source != "" {
		source = h.Source
	}
	auth := Authenticator(StaticActorAuth{})
	if h != nil && h.Auth != nil {
		auth = h.Auth
	}
	actor, err := auth.Authenticate(req)
	if err != nil {
		return commandengine.Request{}, err
	}
	ctx := commandengine.Context{
		Source: source,
		Actor:  actor,
	}
	header := http.Header(nil)
	if req != nil {
		header = req.Header
	}
	for _, item := range []struct {
		header string
		target *modeluuid.UUID
	}{
		{header: "X-Chat-Id", target: &ctx.ChatID},
		{header: "X-Thread-Id", target: &ctx.ThreadID},
		{header: "X-Sandbox-Id", target: &ctx.SandboxID},
	} {
		value := strings.TrimSpace(header.Get(item.header))
		if value == "" {
			continue
		}
		id, err := modeluuid.Parse(value)
		if err != nil {
			return commandengine.Request{}, fmt.Errorf("parse %s: %w", item.header, err)
		}
		*item.target = id
	}
	return commandengine.Request{Context: ctx}, nil
}

func writeResult(w http.ResponseWriter, req *http.Request, stdout string, elapsedMS int64) {
	w.Header().Set("X-Command-Exit-Code", "0")
	w.Header().Set("X-Elapsed-Ms", strconv.FormatInt(elapsedMS, 10))
	if wantsJSON(req) {
		writeJSON(w, http.StatusOK, JSONResponse{ExitCode: 0, Stdout: stdout, ElapsedMS: elapsedMS})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, stdout)
}

func writeError(w http.ResponseWriter, req *http.Request, status int, err error, elapsedMS int64) {
	if status <= 0 {
		status = http.StatusOK
	}
	message := ""
	if err != nil {
		message = err.Error()
	}
	w.Header().Set("X-Command-Exit-Code", "1")
	w.Header().Set("X-Elapsed-Ms", strconv.FormatInt(elapsedMS, 10))
	if wantsJSON(req) {
		writeJSON(w, status, JSONResponse{ExitCode: 1, Stderr: message, Error: message, ElapsedMS: elapsedMS})
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, message)
}

func wantsJSON(req *http.Request) bool {
	if req == nil {
		return false
	}
	return strings.Contains(req.Header.Get("Accept"), "application/json")
}

func wantsSSE(req *http.Request) bool {
	if req == nil {
		return false
	}
	return strings.Contains(req.Header.Get("Accept"), "text/event-stream")
}

func writeJSON(w http.ResponseWriter, status int, resp JSONResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}
