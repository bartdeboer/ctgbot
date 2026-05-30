package v2

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type Client struct {
	BaseURL     string
	HTTPClient  *http.Client
	BearerToken string
}

type RunRequest struct {
	Command   []string
	Query     url.Values
	Stdin     string
	WantJSON  bool
	ChatID    modeluuid.UUID
	ThreadID  modeluuid.UUID
	SandboxID modeluuid.UUID
}

type RunResponse struct {
	StatusCode int
	ExitCode   int
	Text       string
	JSON       JSONResponse
}

func (c *Client) Run(ctx context.Context, req RunRequest) (RunResponse, error) {
	if c == nil {
		return RunResponse{}, fmt.Errorf("missing hostbridgev2 client")
	}
	target, err := c.runURL(req)
	if err != nil {
		return RunResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, target, strings.NewReader(req.Stdin))
	if err != nil {
		return RunResponse{}, err
	}
	if req.Stdin != "" {
		httpReq.Header.Set("Content-Type", "text/plain; charset=utf-8")
	}
	if req.WantJSON {
		httpReq.Header.Set("Accept", "application/json")
	}
	if token := strings.TrimSpace(c.BearerToken); token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+token)
	}
	setUUIDHeader(httpReq.Header, "X-Chat-Id", req.ChatID)
	setUUIDHeader(httpReq.Header, "X-Thread-Id", req.ThreadID)
	setUUIDHeader(httpReq.Header, "X-Sandbox-Id", req.SandboxID)

	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	httpResp, err := httpClient.Do(httpReq)
	if err != nil {
		return RunResponse{}, err
	}
	defer httpResp.Body.Close()
	resp, err := decodeRunResponse(httpResp, req.WantJSON)
	if err != nil {
		return resp, err
	}
	if httpResp.StatusCode >= 400 {
		return resp, fmt.Errorf("hostbridgev2 HTTP %d: %s", httpResp.StatusCode, strings.TrimSpace(resp.Text))
	}
	return resp, nil
}

func (c *Client) runURL(req RunRequest) (string, error) {
	base := strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	if base == "" {
		return "", fmt.Errorf("missing hostbridgev2 base URL")
	}
	if len(req.Command) == 0 {
		return "", fmt.Errorf("missing command")
	}
	parsed, err := url.Parse(base)
	if err != nil {
		return "", fmt.Errorf("parse base URL: %w", err)
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", fmt.Errorf("base URL must include scheme and host")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	rawSegments := make([]string, 0, len(req.Command))
	for _, part := range req.Command {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		rawSegments = append(rawSegments, url.PathEscape(part))
	}
	if len(rawSegments) == 0 {
		return "", fmt.Errorf("missing command")
	}
	target := strings.TrimRight(parsed.String(), "/") + defaultRunPrefix + strings.Join(rawSegments, "/")
	if query := req.Query.Encode(); query != "" {
		target += "?" + query
	}
	return target, nil
}

func decodeRunResponse(httpResp *http.Response, wantJSON bool) (RunResponse, error) {
	if httpResp == nil {
		return RunResponse{}, fmt.Errorf("missing HTTP response")
	}
	body, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return RunResponse{}, fmt.Errorf("read response body: %w", err)
	}
	resp := RunResponse{
		StatusCode: httpResp.StatusCode,
		ExitCode:   parseExitCode(httpResp.Header.Get("X-Command-Exit-Code")),
		Text:       string(body),
	}
	if !wantJSON {
		return resp, nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	if err := dec.Decode(&resp.JSON); err != nil {
		return resp, fmt.Errorf("decode JSON response: %w", err)
	}
	resp.ExitCode = resp.JSON.ExitCode
	resp.Text = resp.JSON.Stdout
	if resp.Text == "" {
		resp.Text = resp.JSON.Stderr
	}
	return resp, nil
}

func parseExitCode(value string) int {
	code, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return code
}

func setUUIDHeader(header http.Header, key string, id modeluuid.UUID) {
	if id.IsNull() {
		return
	}
	header.Set(key, id.String())
}
