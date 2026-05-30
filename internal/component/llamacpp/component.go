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
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	backendruntime "github.com/bartdeboer/ctgbot/internal/runtime/backend"
	"github.com/bartdeboer/ctgbot/internal/workgate"
)

type Component struct {
	registration    coremodel.Component
	componentConfig ComponentConfig
	runtimeConfig   runtimepkg.BindConfig
	backendFactory  backendruntime.Binder
	home            runtimepkg.Home
	resolver        ComponentResolver
	client          *http.Client
	logger          *log.Logger
	runtimeMu       sync.Mutex
	modelStates     map[string]*modelRuntimeState
	inferenceGate   *workgate.Gate
}

type ComponentResolver interface {
	ResolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error)
	ResolveComponent(ctx context.Context, id modeluuid.UUID) (*component.Loaded, error)
}

type modelRuntimeState struct {
	idleStop       *time.Timer
	autoManaged    bool
	activeSessions int
}

var _ component.Component = (*Component)(nil)
var _ component.InferenceEngine = (*Component)(nil)
var _ component.CompletionEngine = (*Component)(nil)
var _ component.InferenceSessionEngine = (*Component)(nil)
var _ component.OpenAIChatEngine = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)

func New(
	ctx context.Context,
	registration coremodel.Component,
	runtimeFactory runtimepkg.Factory,
	home runtimepkg.Home,
	storage repository.Storage,
	resolver ComponentResolver,
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
		runtimeConfig:   runtimeConfig,
		backendFactory:  backendFactory,
		home:            home,
		resolver:        resolver,
		client:          &http.Client{Timeout: 2 * time.Minute},
		logger:          logger,
		inferenceGate:   workgate.New(),
	}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: runtimepkg.ConfigFilename, Required: false, Sensitive: false},
		{RelativePath: ComponentConfigFilename, Required: false, Sensitive: false},
	}
}

func (c *Component) Complete(ctx context.Context, request component.CompletionRequest) (*component.CompletionResult, error) {
	text, err := c.completeWithManagedBackend(ctx, request)
	if err != nil {
		return nil, err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return nil, nil
	}
	return &component.CompletionResult{Final: &coremodel.ThreadMessage{Text: text}}, nil
}

func (c *Component) completeWithManagedBackend(ctx context.Context, request component.CompletionRequest) (string, error) {
	session, err := c.BeginInferenceSession(ctx, component.InferenceSessionOptions{Model: request.Model})
	if err != nil {
		return "", err
	}
	defer func() {
		if err := session.Close(); err != nil {
			c.logf("llamacpp inference session close failed component=%s err=%v", c.registration.Ref(), err)
		}
	}()
	return c.completeWithOptions(ctx, request)
}

func (c *Component) BeginInferenceSession(ctx context.Context, options component.InferenceSessionOptions) (component.InferenceSession, error) {
	return c.beginSession(ctx, options)
}

func (c *Component) BeginOpenAIChatSession(ctx context.Context, options component.InferenceSessionOptions) (component.OpenAIChatSession, error) {
	session, runtime, model, err := c.beginSessionWithRuntime(ctx, options)
	if err != nil {
		return nil, err
	}
	return openAIChatSession{
		InferenceSession: session,
		baseURL:          strings.TrimRight(runtime.BaseURL(), "/") + "/v1",
		model:            model.Name,
	}, nil
}

func (c *Component) beginSession(ctx context.Context, options component.InferenceSessionOptions) (component.InferenceSession, error) {
	session, _, _, err := c.beginSessionWithRuntime(ctx, options)
	return session, err
}

func (c *Component) beginSessionWithRuntime(ctx context.Context, options component.InferenceSessionOptions) (component.InferenceSession, *backendruntime.Runtime, resolvedModel, error) {
	if c == nil {
		return nil, nil, resolvedModel{}, fmt.Errorf("missing llamacpp component")
	}
	runtime, model, err := c.runtimeForModel(options.Model)
	if err != nil {
		return nil, nil, resolvedModel{}, err
	}
	if runtime == nil {
		return nil, nil, resolvedModel{}, fmt.Errorf("missing llamacpp backend runtime")
	}
	c.runtimeMu.Lock()
	defer c.runtimeMu.Unlock()
	state := c.modelStateLocked(model.Name)
	c.cancelIdleStopLocked(state)
	wasRunning, err := isRunning(ctx, runtime)
	if err != nil {
		return nil, nil, resolvedModel{}, err
	}
	if _, err := runtime.Start(ctx); err != nil {
		return nil, nil, resolvedModel{}, err
	}
	if !wasRunning {
		state.autoManaged = true
	}
	state.activeSessions++
	modelName := model.Name
	return &inferenceSession{
		close: func() error {
			return c.releaseInferenceSession(modelName, options.IdleTimeout)
		},
	}, runtime, model, nil
}

func (c *Component) isRunning(ctx context.Context) (bool, error) {
	runtime, _, err := c.runtimeForModel("")
	if err != nil {
		return false, err
	}
	return isRunning(ctx, runtime)
}

func isRunning(ctx context.Context, runtime *backendruntime.Runtime) (bool, error) {
	if runtime == nil {
		return false, nil
	}
	status, err := runtime.Status(ctx)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(status.State) == "running", nil
}

func serviceSpec(config resolvedModel) backendruntime.ServiceSpec {
	modelDir := filepath.Dir(config.ModelPath)
	cmd := []string{
		"-m", "/models/" + filepath.Base(config.ModelPath),
		"--host", "0.0.0.0",
		"--port", "8080",
		"--ctx-size", strconv.Itoa(config.ContextSize),
		"--gpu-layers", strconv.Itoa(config.GPULayers),
	}
	if cleanModelMode(config.Mode) == "embedding" {
		cmd = append(cmd, "--embedding")
		if strings.TrimSpace(config.Pooling) != "" {
			cmd = append(cmd, "--pooling", strings.TrimSpace(config.Pooling))
		}
		if config.UBatchSize > 0 {
			cmd = append(cmd, "-ub", strconv.Itoa(config.UBatchSize))
		}
	} else {
		cmd = append(cmd, "--jinja")
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
	portBinding := fmt.Sprintf("127.0.0.1:%d:8080", config.HostPort)
	if config.ExposeToSandboxes {
		portBinding = fmt.Sprintf("%d:8080", config.HostPort)
	}
	return backendruntime.ServiceSpec{
		BaseURL:   baseURL,
		HealthURL: baseURL + "/health",
		Ports:     []string{portBinding},
		Mounts:    mounts,
		Cmd:       cmd,
	}
}

func (c *Component) stopAfterInference(modelName string) error {
	runtime, _, err := c.runtimeForModel(modelName)
	if err != nil {
		return err
	}
	if runtime == nil {
		return nil
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := runtime.Stop(stopCtx); err != nil {
		c.logf("llamacpp stop-after-completion failed component=%s err=%v", c.registration.Ref(), err)
		return err
	}
	return nil
}

func (c *Component) releaseInferenceSession(modelName string, idleTimeout time.Duration) error {
	if c == nil {
		return nil
	}
	c.runtimeMu.Lock()
	state := c.modelStateLocked(modelName)
	if state.activeSessions > 0 {
		state.activeSessions--
	}
	if state.activeSessions > 0 {
		c.runtimeMu.Unlock()
		return nil
	}
	if !state.autoManaged || c.componentConfig.KeepRunning {
		c.runtimeMu.Unlock()
		return nil
	}
	if idleTimeout <= 0 {
		state.autoManaged = false
		c.runtimeMu.Unlock()
		return c.stopAfterInference(modelName)
	}
	c.cancelIdleStopLocked(state)
	var timer *time.Timer
	timer = time.AfterFunc(idleTimeout, func() {
		c.runtimeMu.Lock()
		state := c.modelStateLocked(modelName)
		if state.idleStop != timer {
			c.runtimeMu.Unlock()
			return
		}
		state.idleStop = nil
		state.autoManaged = false
		c.runtimeMu.Unlock()
		if err := c.stopAfterInference(modelName); err != nil {
			c.logf("llamacpp idle stop failed component=%s err=%v", c.registration.Ref(), err)
		}
	})
	state.idleStop = timer
	c.runtimeMu.Unlock()
	return nil
}

func (c *Component) modelStateLocked(modelName string) *modelRuntimeState {
	if c.modelStates == nil {
		c.modelStates = map[string]*modelRuntimeState{}
	}
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		modelName = "default"
	}
	state := c.modelStates[modelName]
	if state == nil {
		state = &modelRuntimeState{}
		c.modelStates[modelName] = state
	}
	return state
}

func (c *Component) cancelIdleStopLocked(state *modelRuntimeState) {
	if state == nil || state.idleStop == nil {
		return
	}
	state.idleStop.Stop()
	state.idleStop = nil
}

type inferenceSession struct {
	once  sync.Once
	close func() error
	err   error
}

func (s *inferenceSession) Close() error {
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

func (c *Component) completeWithOptions(ctx context.Context, request component.CompletionRequest) (string, error) {
	runtime, model, err := c.runtimeForModel(request.Model)
	if err != nil {
		return "", err
	}
	if cleanModelMode(model.Mode) != "completion" {
		return "", fmt.Errorf("llama.cpp model %s is not configured for chat completions", model.Name)
	}
	release, err := c.acquireInference(ctx, model.Name)
	if err != nil {
		return "", err
	}
	defer release()
	maxTokens := model.MaxTokens
	if request.MaxOutputTokens > 0 {
		maxTokens = request.MaxOutputTokens
	}
	temperature := model.Temperature
	if request.Temperature > 0 {
		temperature = request.Temperature
	}
	body := cloneProviderOptions(request.ProviderOptions)
	body["model"] = model.Name
	body["messages"] = completionPromptToChat(request.Prompt)
	if maxTokens > 0 {
		body["max_tokens"] = maxTokens
	}
	if temperature > 0 {
		body["temperature"] = temperature
	}
	if strings.EqualFold(strings.TrimSpace(request.ResponseFormat), "json") {
		body["response_format"] = completionResponseFormat{Type: "json_object"}
	}
	applyReasoningMode(body, request.Reasoning)
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, runtime.BaseURL()+"/v1/chat/completions", bytes.NewReader(data))
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

func cloneProviderOptions(options map[string]any) map[string]any {
	if len(options) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(options))
	for key, value := range options {
		out[strings.TrimSpace(key)] = cloneProviderOptionValue(value)
	}
	return out
}

func cloneProviderOptionValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneProviderOptions(typed)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = cloneProviderOptionValue(item)
		}
		return out
	default:
		return value
	}
}

func applyReasoningMode(body map[string]any, mode component.ReasoningMode) {
	var enableThinking any
	switch mode {
	case component.ReasoningEnabled:
		enableThinking = true
	case component.ReasoningDisabled:
		enableThinking = false
	default:
		return
	}
	kwargs, _ := body["chat_template_kwargs"].(map[string]any)
	if kwargs == nil {
		kwargs = map[string]any{}
	}
	kwargs["enable_thinking"] = enableThinking
	body["chat_template_kwargs"] = kwargs
}

func (c *Component) acquireInference(ctx context.Context, modelName string) (func(), error) {
	if c == nil || c.inferenceGate == nil {
		return func() {}, nil
	}
	return c.inferenceGate.Acquire(ctx, cleanModelName(modelName), c.componentConfig.MaxConcurrent)
}

func (c *Component) logf(format string, args ...any) {
	if c != nil && c.logger != nil {
		c.logger.Printf(format, args...)
	}
}

func (c *Component) runtimeForModel(name string) (*backendruntime.Runtime, resolvedModel, error) {
	model, err := c.resolveModel(name)
	if err != nil {
		return nil, resolvedModel{}, err
	}
	if c.backendFactory == nil {
		return nil, resolvedModel{}, fmt.Errorf("missing llama.cpp backend factory")
	}
	registration := c.registration
	if model.Name != "" && model.Name != "default" {
		registration.Name = c.registration.Name + "-" + model.Name
	}
	return c.backendFactory.BindBackend(registration, c.home, c.runtimeConfig, serviceSpec(model)), model, nil
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

type openAIChatSession struct {
	component.InferenceSession
	baseURL string
	model   string
	apiKey  string
}

func (s openAIChatSession) BaseURL() string { return strings.TrimSpace(s.baseURL) }
func (s openAIChatSession) Model() string   { return strings.TrimSpace(s.model) }
func (s openAIChatSession) APIKey() string  { return strings.TrimSpace(s.apiKey) }
