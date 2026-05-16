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
	"sync"
	"time"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/containerengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	backendruntime "github.com/bartdeboer/ctgbot/internal/runtime/backend"
)

type Component struct {
	registration    coremodel.Component
	componentConfig ComponentConfig
	runtime         *backendruntime.Runtime
	client          *http.Client
	logger          *log.Logger
	runtimeMu       sync.Mutex
	idleStop        *time.Timer
	autoManaged     bool
	activeSessions  int
}

var _ component.Component = (*Component)(nil)
var _ component.CompletionProvider = (*Component)(nil)
var _ component.CompletionSessionProvider = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)

func New(
	ctx context.Context,
	registration coremodel.Component,
	runtimeFactory runtimepkg.Factory,
	home runtimepkg.Home,
	storage repository.Storage,
	logger *log.Logger,
) (component.Component, error) {
	_, _ = ctx, storage
	backendFactory, ok := runtimeFactory.(backendruntime.Binder)
	if !ok {
		return nil, fmt.Errorf("llamacpp requires backend runtime, got %T", runtimeFactory)
	}
	runtimeConfig, err := loadRuntimeConfig(home.Path)
	if err != nil {
		return nil, err
	}
	componentConfig, err := loadComponentConfig(home.Path, registration.Name)
	if err != nil {
		return nil, err
	}
	return &Component{
		registration:    registration,
		componentConfig: componentConfig,
		runtime:         backendFactory.BindBackend(registration, home, runtimeConfig, serviceSpec(componentConfig)),
		client:          &http.Client{Timeout: 2 * time.Minute},
		logger:          logger,
	}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: runtimepkg.ConfigFilename, Required: false, Sensitive: false},
		{RelativePath: ComponentConfigFilename, Required: false, Sensitive: false},
	}
}

func (c *Component) HandleCompletion(ctx context.Context, request component.CompletionRequest) (*component.CompletionResult, error) {
	text, err := c.completeWithManagedBackend(ctx, completionPromptToChat(request.Prompt), request.MaxOutputTokens, request.ResponseFormat)
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	return &component.CompletionResult{Final: &coremodel.ThreadMessage{Text: text}}, nil
}

func (c *Component) completeWithManagedBackend(ctx context.Context, messages []chatMessage, maxOutputTokens int, responseFormat string) (string, error) {
	session, err := c.BeginCompletionSession(ctx, component.CompletionSessionOptions{})
	if err != nil {
		return "", err
	}
	defer func() {
		if err := session.Close(); err != nil {
			c.logf("llamacpp completion session close failed component=%s err=%v", c.registration.Ref(), err)
		}
	}()
	return c.completeWithOptions(ctx, messages, maxOutputTokens, responseFormat)
}

func (c *Component) BeginCompletionSession(ctx context.Context, options component.CompletionSessionOptions) (component.CompletionSession, error) {
	if c == nil {
		return nil, fmt.Errorf("missing llamacpp component")
	}
	if c.runtime == nil {
		return nil, fmt.Errorf("missing llamacpp backend runtime")
	}
	c.runtimeMu.Lock()
	defer c.runtimeMu.Unlock()
	c.cancelIdleStopLocked()
	wasRunning, err := c.isRunning(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := c.runtime.Start(ctx); err != nil {
		return nil, err
	}
	if !wasRunning {
		c.autoManaged = true
	}
	c.activeSessions++
	return &completionSession{
		close: func() error {
			return c.releaseCompletionSession(options.IdleTimeout)
		},
	}, nil
}

func (c *Component) isRunning(ctx context.Context) (bool, error) {
	if c == nil || c.runtime == nil {
		return false, nil
	}
	status, err := c.runtime.Status(ctx)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(status.State) == "running", nil
}

func serviceSpec(config ComponentConfig) backendruntime.ServiceSpec {
	modelDir := filepath.Dir(config.ModelPath)
	cmd := []string{
		"-m", "/models/" + filepath.Base(config.ModelPath),
		"--host", "0.0.0.0",
		"--port", "8080",
		"--ctx-size", strconv.Itoa(config.ContextSize),
		"--gpu-layers", strconv.Itoa(config.GPULayers),
		"--jinja",
	}
	mounts := []containerengine.Mount{{Source: modelDir, Target: "/models", ReadOnly: true}}
	if mmprojPath := strings.TrimSpace(config.MMProjPath); mmprojPath != "" {
		if filepath.Dir(mmprojPath) == modelDir {
			cmd = append(cmd, "--mmproj", "/models/"+filepath.Base(mmprojPath))
		} else {
			mounts = append(mounts, containerengine.Mount{
				Source:   filepath.Dir(mmprojPath),
				Target:   "/mmproj",
				ReadOnly: true,
			})
			cmd = append(cmd, "--mmproj", "/mmproj/"+filepath.Base(mmprojPath))
		}
	}
	baseURL := fmt.Sprintf("http://127.0.0.1:%d", config.HostPort)
	return backendruntime.ServiceSpec{
		BaseURL:   baseURL,
		HealthURL: baseURL + "/health",
		Ports:     []string{fmt.Sprintf("127.0.0.1:%d:8080", config.HostPort)},
		Mounts:    mounts,
		Cmd:       cmd,
	}
}

func (c *Component) stopAfterCompletion() error {
	if c == nil || c.runtime == nil {
		return nil
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.runtime.Stop(stopCtx); err != nil {
		c.logf("llamacpp stop-after-completion failed component=%s err=%v", c.registration.Ref(), err)
		return err
	}
	return nil
}

func (c *Component) releaseCompletionSession(idleTimeout time.Duration) error {
	if c == nil {
		return nil
	}
	c.runtimeMu.Lock()
	if c.activeSessions > 0 {
		c.activeSessions--
	}
	if c.activeSessions > 0 {
		c.runtimeMu.Unlock()
		return nil
	}
	if !c.autoManaged || c.componentConfig.KeepRunning {
		c.runtimeMu.Unlock()
		return nil
	}
	if idleTimeout <= 0 {
		c.autoManaged = false
		c.runtimeMu.Unlock()
		return c.stopAfterCompletion()
	}
	c.cancelIdleStopLocked()
	var timer *time.Timer
	timer = time.AfterFunc(idleTimeout, func() {
		c.runtimeMu.Lock()
		if c.idleStop != timer {
			c.runtimeMu.Unlock()
			return
		}
		c.idleStop = nil
		c.autoManaged = false
		c.runtimeMu.Unlock()
		if err := c.stopAfterCompletion(); err != nil {
			c.logf("llamacpp idle stop failed component=%s err=%v", c.registration.Ref(), err)
		}
	})
	c.idleStop = timer
	c.runtimeMu.Unlock()
	return nil
}

func (c *Component) cancelIdleStopLocked() {
	if c == nil || c.idleStop == nil {
		return
	}
	c.idleStop.Stop()
	c.idleStop = nil
}

type completionSession struct {
	once  sync.Once
	close func() error
	err   error
}

func (s *completionSession) Close() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() {
		if s.close != nil {
			s.err = s.close()
		}
	})
	return s.err
}

func (c *Component) completeWithOptions(ctx context.Context, messages []chatMessage, maxOutputTokens int, responseFormat string) (string, error) {
	maxTokens := c.componentConfig.MaxTokens
	if maxOutputTokens > 0 {
		maxTokens = maxOutputTokens
	}
	body := completionRequest{
		Model:       c.registration.Name,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: c.componentConfig.Temperature,
	}
	if strings.EqualFold(strings.TrimSpace(responseFormat), "json") {
		body.ResponseFormat = &completionResponseFormat{Type: "json_object"}
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
	if c == nil || c.runtime == nil {
		return ""
	}
	return c.runtime.BaseURL()
}

func (c *Component) logf(format string, args ...any) {
	if c != nil && c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

type completionRequest struct {
	Model          string                    `json:"model"`
	Messages       []chatMessage             `json:"messages"`
	MaxTokens      int                       `json:"max_tokens,omitempty"`
	Temperature    float64                   `json:"temperature,omitempty"`
	ResponseFormat *completionResponseFormat `json:"response_format,omitempty"`
}

type completionResponseFormat struct {
	Type string `json:"type"`
}

type completionResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}
