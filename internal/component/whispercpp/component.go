package whispercpp

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/containerengine"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/durationparse"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/workgate"
)

type ComponentResolver interface {
	ResolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error)
	ResolveComponent(ctx context.Context, id modeluuid.UUID) (*component.Loaded, error)
}

type Component struct {
	registration   coremodel.Component
	sandboxes      sandboxengine.RuntimeManager
	home           runtimepkg.Home
	runtimeConfig  runtimepkg.BindConfig
	runtimeHome    string
	config         ComponentConfig
	resolver       ComponentResolver
	transcribeGate *workgate.Gate
}

var _ component.Component = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)
var _ component.Transcriber = (*Component)(nil)

func New(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage, resolver ComponentResolver) (component.Component, error) {
	_, _ = ctx, storage
	runtimeConfig, err := loadRuntimeConfig(home.Path)
	if err != nil {
		return nil, err
	}
	provider, ok := runtime.(sandboxengine.Provider)
	if !ok || provider.SandboxManager() == nil {
		return nil, fmt.Errorf("whispercpp requires a sandbox-capable runtime")
	}
	config, err := loadComponentConfig(home.Path)
	if err != nil {
		return nil, err
	}
	return &Component{
		registration:   registration,
		sandboxes:      provider.SandboxManager(),
		home:           home,
		runtimeConfig:  runtimeConfig,
		runtimeHome:    runtime.RuntimeComponentHomePath(registration, home),
		config:         config,
		resolver:       resolver,
		transcribeGate: workgate.New(),
	}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: ComponentConfigFilename, Required: false, Sensitive: false},
		{RelativePath: runtimepkg.ConfigFilename, Required: false, Sensitive: false},
	}
}

func (c *Component) Transcribe(ctx context.Context, req component.TranscriptionRequest) (component.TranscriptionResult, error) {
	if c == nil || c.sandboxes == nil {
		return component.TranscriptionResult{}, fmt.Errorf("missing whispercpp runtime")
	}
	model, err := c.resolveModel(ctx, firstNonEmpty(req.Model, c.config.DefaultModel))
	if err != nil {
		return component.TranscriptionResult{}, err
	}
	if strings.TrimSpace(model.Path) == "" {
		return component.TranscriptionResult{}, fmt.Errorf("missing whispercpp model path")
	}
	if model.Mode != "" && model.Mode != component.ModelModeASR {
		return component.TranscriptionResult{}, fmt.Errorf("model %s is mode=%s, want asr", model.Name, model.Mode)
	}
	if len(req.Media.Content) == 0 {
		return component.TranscriptionResult{}, fmt.Errorf("missing audio content")
	}
	release, err := c.acquireTranscription(ctx, model.Name)
	if err != nil {
		return component.TranscriptionResult{}, err
	}
	defer release()
	work, cleanup, err := c.prepareWorkdir("transcribe-*")
	if err != nil {
		return component.TranscriptionResult{}, err
	}
	defer cleanup()

	inputHost := filepath.Join(work.host, inputFilename(req.Media))
	wavHost := filepath.Join(work.host, "input.wav")
	transcriptHost := filepath.Join(work.host, "transcript.txt")
	if err := os.WriteFile(inputHost, req.Media.Content, 0o600); err != nil {
		return component.TranscriptionResult{}, err
	}

	modelDir := filepath.Dir(model.Path)
	modelRuntime := filepath.Join(workspaceRuntimePath, filepath.Base(model.Path))
	values := map[string]string{
		"input":         filepath.Join(work.runtime, filepath.Base(inputHost)),
		"wav":           filepath.Join(work.runtime, filepath.Base(wavHost)),
		"model":         modelRuntime,
		"output_prefix": filepath.Join(work.runtime, "transcript"),
	}
	spec, err := c.sandboxSpec(modelDir)
	if err != nil {
		return component.TranscriptionResult{}, err
	}
	session, err := c.sandboxes.BeginSession(ctx, spec, c.sessionOptions())
	if err != nil {
		return component.TranscriptionResult{}, err
	}
	defer session.Close()

	if err := c.run(ctx, session, c.config.FFMpegCommand, renderArgs(defaultFFMpegArgs(), values)); err != nil {
		return component.TranscriptionResult{}, err
	}
	requestedLanguage := firstNonEmpty(req.Language, c.config.Language)
	args := c.whisperArgs(values, requestedLanguage)
	out, err := session.CombinedOutput(ctx, c.config.WhisperCommand, args...)
	if err != nil {
		return component.TranscriptionResult{}, fmt.Errorf("whispercpp command: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	textBytes, err := os.ReadFile(transcriptHost)
	if err != nil {
		return component.TranscriptionResult{}, fmt.Errorf("read whispercpp transcript: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	text := strings.TrimSpace(string(textBytes))
	if text == "" {
		return component.TranscriptionResult{}, fmt.Errorf("whispercpp returned empty transcript")
	}
	language := requestedLanguage
	if language == "" || strings.EqualFold(language, "auto") {
		language = detectedLanguageFromWhisperOutput(string(out))
	}
	return component.TranscriptionResult{Text: text, Language: language, Model: model.Name}, nil
}

func (c *Component) acquireTranscription(ctx context.Context, modelName string) (func(), error) {
	if c == nil || c.transcribeGate == nil {
		return func() {}, nil
	}
	return c.transcribeGate.Acquire(ctx, strings.TrimSpace(modelName), c.config.MaxConcurrent)
}

func (c *Component) sandboxSpec(modelDir string) (sandboxengine.SandboxSpec, error) {
	securityOpts, err := containerengine.SeccompSecurityOpts(c.runtimeConfig.Seccomp)
	if err != nil {
		return sandboxengine.SandboxSpec{}, err
	}
	name := sandboxengine.SafeName("ctgbot-"+c.registration.Ref()+"-"+workspaceKey(modelDir), "ctgbot-runtime")
	return *sandboxengine.NewBuilder(name).
		Image(firstNonEmpty(c.runtimeConfig.Image, DefaultImage)).
		Entrypoint(c.runtimeConfig.Entrypoint).
		Workdir(workspaceRuntimePath).
		UserMode("host").
		GPUs(c.runtimeConfig.GPUs).
		Env(c.runtimeConfig.Env).
		Mounts([]sandboxengine.Mount{
			{Source: c.home.Path, Target: c.runtimeHome},
			{Source: modelDir, Target: workspaceRuntimePath, ReadOnly: true},
		}).
		SecurityOpts(securityOpts).
		Cmd(idleCmd(c.runtimeConfig.Cmd)).
		RecreateStopped(true).
		Build(), nil
}

func (c *Component) sessionOptions() sandboxengine.SessionOptions {
	timeout, _ := durationparse.Parse(c.runtimeConfig.IdleTimeout, time.Second)
	return sandboxengine.SessionOptions{IdleTimeout: timeout}
}

func (c *Component) run(ctx context.Context, session *sandboxengine.Session, name string, args []string) error {
	out, err := session.CombinedOutput(ctx, name, args...)
	if err != nil {
		return fmt.Errorf("%s: %w\n%s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (c *Component) whisperArgs(values map[string]string, language string) []string {
	args := c.config.WhisperArgs
	if len(args) == 0 {
		args = defaultWhisperArgs()
	}
	args = renderArgs(args, values)
	if !hasArg(args, "-l") && !hasArg(args, "--language") {
		args = append(args, "-l", firstNonEmpty(language, "auto"))
	}
	if c.config.Threads > 0 {
		args = append(args, "-t", strconv.Itoa(c.config.Threads))
	}
	return args
}

func defaultFFMpegArgs() []string {
	return []string{"-hide_banner", "-y", "-i", "{{input}}", "-vn", "-ar", "16000", "-ac", "1", "-c:a", "pcm_s16le", "{{wav}}"}
}

func defaultWhisperArgs() []string {
	return []string{"-m", "{{model}}", "-f", "{{wav}}", "-fa", "-otxt", "-of", "{{output_prefix}}"}
}

var whisperDetectedLanguagePattern = regexp.MustCompile(`(?i)auto-detected language:\s*([a-z]{2,3})\b`)

func detectedLanguageFromWhisperOutput(output string) string {
	match := whisperDetectedLanguagePattern.FindStringSubmatch(output)
	if len(match) < 2 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(match[1]))
}

type workdir struct {
	host    string
	runtime string
}

func (c *Component) prepareWorkdir(pattern string) (workdir, func(), error) {
	host, err := os.MkdirTemp(c.home.Path, pattern)
	if err != nil {
		return workdir{}, func() {}, err
	}
	runtime := filepath.Join(c.runtimeHome, filepath.Base(host))
	return workdir{host: host, runtime: runtime}, func() { _ = os.RemoveAll(host) }, nil
}

func inputFilename(media message.Media) string {
	name := strings.TrimSpace(media.Filename)
	if name == "" {
		return "input.audio"
	}
	return filepath.Base(name)
}

func renderArgs(args []string, values map[string]string) []string {
	out := make([]string, 0, len(args))
	for _, arg := range args {
		for key, value := range values {
			arg = strings.ReplaceAll(arg, "{{"+key+"}}", value)
		}
		out = append(out, arg)
	}
	return out
}

func hasArg(args []string, names ...string) bool {
	for _, arg := range args {
		arg = strings.TrimSpace(arg)
		for _, name := range names {
			if arg == name {
				return true
			}
		}
	}
	return false
}

const workspaceRuntimePath = "/workspace"

func idleCmd(cmd []string) []string {
	if len(cmd) > 0 {
		return append([]string{}, cmd...)
	}
	return []string{"tail", "-f", "/dev/null"}
}

func workspaceKey(value string) string {
	sum := sha1.Sum([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])[:12]
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return commandDefinitions()
}
