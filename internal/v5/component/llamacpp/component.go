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
	v5backend "github.com/bartdeboer/ctgbot/internal/v5/runtime/backend"
)

type Component struct {
	registration    coremodel.Component
	componentConfig ComponentConfig
	runtime         *v5backend.Runtime
	client          *http.Client
	logger          *log.Logger
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
	_, _ = ctx, storage
	backendFactory, ok := runtimeFactory.(v5backend.Binder)
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
		{RelativePath: v5runtime.ConfigFilename, Required: false, Sensitive: false},
		{RelativePath: ComponentConfigFilename, Required: false, Sensitive: false},
	}
}

func (c *Component) HandleCompletion(ctx context.Context, request component.CompletionRequest) (*component.CompletionResult, error) {
	if c == nil {
		return nil, fmt.Errorf("missing llamacpp component")
	}
	if c.runtime == nil {
		return nil, fmt.Errorf("missing llamacpp backend runtime")
	}
	wasRunning, err := c.isRunning(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := c.runtime.Start(ctx); err != nil {
		return nil, err
	}
	autoStarted := !wasRunning
	if autoStarted && !c.componentConfig.KeepRunning {
		defer c.stopAfterCompletion()
	}
	text, err := c.complete(ctx, completionPromptToChat(request.Prompt))
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	return &component.CompletionResult{Final: &coremodel.ThreadMessage{Text: text}}, nil
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

func serviceSpec(config ComponentConfig) v5backend.ServiceSpec {
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
	return v5backend.ServiceSpec{
		BaseURL:   baseURL,
		HealthURL: baseURL + "/health",
		Ports:     []string{fmt.Sprintf("127.0.0.1:%d:8080", config.HostPort)},
		Mounts:    mounts,
		Cmd:       cmd,
	}
}

func (c *Component) stopAfterCompletion() {
	if c == nil || c.runtime == nil {
		return
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.runtime.Stop(stopCtx); err != nil {
		c.logf("llamacpp stop-after-completion failed component=%s err=%v", c.registration.Ref(), err)
	}
}

func (c *Component) complete(ctx context.Context, messages []chatMessage) (string, error) {
	body := completionRequest{
		Model:       c.registration.Name,
		Messages:    messages,
		MaxTokens:   c.componentConfig.MaxTokens,
		Temperature: c.componentConfig.Temperature,
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
