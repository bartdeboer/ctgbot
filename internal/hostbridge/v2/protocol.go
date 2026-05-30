package v2

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

const (
	defaultRunPrefix = "/v2/run/"
)

type CommandRunner interface {
	Run(ctx context.Context, base commandengine.Request, argv []string) (commandengine.Result, error)
}

type Handler struct {
	Runner CommandRunner
}

type JSONResponse struct {
	CommandID string `json:"command_id,omitempty"`
	ExitCode  int    `json:"exit_code"`
	Stdout    string `json:"stdout,omitempty"`
	Stderr    string `json:"stderr,omitempty"`
	Error     string `json:"error,omitempty"`
	ElapsedMS int64  `json:"elapsed_ms"`
}

func NewHandler(runner CommandRunner) *Handler {
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
	argv, err := argvFromRequest(req)
	if err != nil {
		writeError(w, req, http.StatusBadRequest, err, 0)
		return
	}
	base, err := baseRequestFromHeaders(req.Header)
	if err != nil {
		writeError(w, req, http.StatusBadRequest, err, 0)
		return
	}
	started := time.Now()
	result, err := h.Runner.Run(req.Context(), base, argv)
	elapsed := time.Since(started).Milliseconds()
	if err != nil {
		writeError(w, req, http.StatusOK, err, elapsed)
		return
	}
	writeResult(w, req, result.Text, elapsed)
}

func argvFromRequest(req *http.Request) ([]string, error) {
	if req == nil || req.URL == nil {
		return nil, fmt.Errorf("missing request URL")
	}
	path := strings.TrimPrefix(req.URL.EscapedPath(), defaultRunPrefix)
	if path == req.URL.EscapedPath() || strings.Trim(path, "/") == "" {
		return nil, fmt.Errorf("expected path %s<command>", defaultRunPrefix)
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
	argv = append(argv, flagsFromQuery(req.URL.Query())...)
	if req.Body != nil {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			return nil, fmt.Errorf("read request body: %w", err)
		}
		if len(body) > 0 {
			argv = append(argv, string(body))
		}
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("missing command")
	}
	return argv, nil
}

func flagsFromQuery(values url.Values) []string {
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

func baseRequestFromHeaders(header http.Header) (commandengine.Request, error) {
	ctx := commandengine.Context{
		Source: commandengine.SourceHostbridge,
		Actor:  commandengine.Actor{ID: firstHeader(header, "X-Actor-Id", "hostbridgev2"), Roles: []simplerbac.Role{simplerbac.RoleAgent}},
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

func firstHeader(header http.Header, key string, fallback string) string {
	value := strings.TrimSpace(header.Get(key))
	if value == "" {
		return fallback
	}
	return value
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

func writeJSON(w http.ResponseWriter, status int, resp JSONResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}
