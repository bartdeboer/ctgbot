package llamacpp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/containerengine"
	"github.com/bartdeboer/ctgbot/internal/v5/component"
	"github.com/bartdeboer/ctgbot/internal/v5/coremodel"
	"github.com/bartdeboer/ctgbot/internal/v5/repository"
	v5runtime "github.com/bartdeboer/ctgbot/internal/v5/runtime"
)

type Component struct {
	registration coremodel.Component
	home         v5runtime.Home
	profile      Profile
	containers   *containerengine.Manager
	client       *http.Client
	logger       *log.Logger
}

var _ component.Component = (*Component)(nil)
var _ component.CompletionAgent = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)

func New(
	ctx context.Context,
	registration coremodel.Component,
	runtimeFactory v5runtime.Factory,
	home v5runtime.Home,
	storage repository.Storage,
	logger *log.Logger,
) (component.Component, error) {
	_, _, _ = ctx, runtimeFactory, storage
	profile, err := loadProfile(home.Path, registration.Name)
	if err != nil {
		return nil, err
	}
	return &Component{
		registration: registration,
		home:         home,
		profile:      profile,
		containers:   containerengine.NewManager(logger),
		client:       &http.Client{Timeout: 2 * time.Minute},
		logger:       logger,
	}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{{RelativePath: "config.json", Required: false, Sensitive: false}}
}

func (c *Component) HandleCompletion(ctx context.Context, request component.CompletionRequest) (*component.CompletionResult, error) {
	if c == nil {
		return nil, fmt.Errorf("missing llamacpp component")
	}
	if err := c.ensureServer(ctx); err != nil {
		return nil, err
	}
	text, err := c.complete(ctx, brokerMessagesToChat(request.Messages))
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	return &component.CompletionResult{Final: &coremodel.ThreadMessage{Text: text}}, nil
}

func (c *Component) ensureServer(ctx context.Context) error {
	container := c.containers.Container(c.containerName())
	state, err := container.InspectState(ctx)
	if err != nil {
		return err
	}
	if state == containerengine.StateMissing {
		if _, err := c.containers.Create(ctx, c.containerSpec()); err != nil {
			return err
		}
	}
	if state != containerengine.StateRunning {
		if err := container.Start(ctx); err != nil {
			return err
		}
	}
	return c.waitReady(ctx)
}

func (c *Component) containerSpec() containerengine.ContainerSpec {
	modelDir := filepath.Dir(c.profile.ModelPath)
	cmd := []string{
		"-m", "/models/" + filepath.Base(c.profile.ModelPath),
		"--host", "0.0.0.0",
		"--port", "8080",
		"--ctx-size", strconv.Itoa(c.profile.ContextSize),
		"--gpu-layers", strconv.Itoa(c.profile.GPULayers),
		"--jinja",
	}
	mounts := []containerengine.Mount{{Source: modelDir, Target: "/models", ReadOnly: true}}
	if strings.TrimSpace(c.profile.MMProjPath) != "" {
		cmd = append(cmd, "--mmproj", "/models/"+filepath.Base(c.profile.MMProjPath))
	}
	return containerengine.ContainerSpec{
		Name:   c.containerName(),
		Image:  c.profile.Image,
		GPUs:   "all",
		Ports:  []string{fmt.Sprintf("127.0.0.1:%d:8080", c.profile.HostPort)},
		Mounts: mounts,
		Cmd:    cmd,
	}
}

func (c *Component) waitReady(ctx context.Context) error {
	url := c.baseURL() + "/health"
	deadline := time.Now().Add(2 * time.Minute)
	var lastErr error
	for time.Now().Before(deadline) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		resp, err := c.client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				return nil
			}
			lastErr = fmt.Errorf("health status %s", resp.Status)
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
	return fmt.Errorf("llamacpp server not ready: %w", lastErr)
}

func (c *Component) complete(ctx context.Context, messages []chatMessage) (string, error) {
	body := completionRequest{
		Model:       c.registration.Name,
		Messages:    messages,
		MaxTokens:   c.profile.MaxTokens,
		Temperature: c.profile.Temperature,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL()+"/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("llamacpp completion status %s: %s", resp.Status, strings.TrimSpace(string(payload)))
	}
	var decoded completionResponse
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return "", err
	}
	if len(decoded.Choices) == 0 {
		return "", nil
	}
	return decoded.Choices[0].Message.Content, nil
}

func (c *Component) baseURL() string {
	return fmt.Sprintf("http://127.0.0.1:%d", c.profile.HostPort)
}

func (c *Component) containerName() string {
	return "ctgbot-v5-llamacpp-" + safeName(c.registration.Name)
}

func safeName(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	prevDash := false
	for _, r := range value {
		ok := r >= 'a' && r <= 'z' || r >= '0' && r <= '9'
		if ok {
			b.WriteRune(r)
			prevDash = false
			continue
		}
		if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "default"
	}
	return out
}

type completionRequest struct {
	Model       string        `json:"model"`
	Messages    []chatMessage `json:"messages"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Temperature float64       `json:"temperature,omitempty"`
}

type completionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
