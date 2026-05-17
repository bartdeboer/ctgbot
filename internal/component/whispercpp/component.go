package whispercpp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
	"github.com/bartdeboer/ctgbot/internal/repository"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
)

type ComponentResolver interface {
	ResolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error)
	ResolveComponent(ctx context.Context, id modeluuid.UUID) (*component.Loaded, error)
}

type Component struct {
	registration coremodel.Component
	runtime      runtimepkg.Runtime
	home         runtimepkg.Home
	config       ComponentConfig
	resolver     ComponentResolver
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
	config, err := loadComponentConfig(home.Path)
	if err != nil {
		return nil, err
	}
	return &Component{
		registration: registration,
		runtime:      runtime.Bind(registration, home, runtimeConfig),
		home:         home,
		config:       config,
		resolver:     resolver,
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
	if c == nil || c.runtime == nil {
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
	work, cleanup, err := c.prepareWorkdir("transcribe-*")
	if err != nil {
		return component.TranscriptionResult{}, err
	}
	defer cleanup()

	inputHost := filepath.Join(work.host, inputFilename(req.Media))
	wavHost := filepath.Join(work.host, "input.wav")
	if err := os.WriteFile(inputHost, req.Media.Content, 0o600); err != nil {
		return component.TranscriptionResult{}, err
	}

	modelDir := filepath.Dir(model.Path)
	modelRuntime := filepath.Join(c.runtime.RuntimeWorkspacePath(modelDir), filepath.Base(model.Path))
	values := map[string]string{
		"input": filepath.Join(work.runtime, filepath.Base(inputHost)),
		"wav":   filepath.Join(work.runtime, filepath.Base(wavHost)),
		"model": modelRuntime,
	}
	if err := c.run(ctx, req.ThreadID, modelDir, c.config.FFMpegCommand, renderArgs(defaultFFMpegArgs(), values)); err != nil {
		return component.TranscriptionResult{}, err
	}
	args := c.whisperArgs(values, firstNonEmpty(req.Language, c.config.Language))
	out, err := c.runtime.CombinedOutput(ctx, modelDir, req.ThreadID, nil, c.config.WhisperCommand, args...)
	if err != nil {
		return component.TranscriptionResult{}, fmt.Errorf("whispercpp command: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	text := cleanTranscript(string(out))
	if text == "" {
		return component.TranscriptionResult{}, fmt.Errorf("whispercpp returned empty transcript")
	}
	return component.TranscriptionResult{Text: text, Language: firstNonEmpty(req.Language, c.config.Language), Model: model.Name}, nil
}

func (c *Component) run(ctx context.Context, threadID modeluuid.UUID, workspacePath string, name string, args []string) error {
	out, err := c.runtime.CombinedOutput(ctx, workspacePath, threadID, nil, name, args...)
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
	if strings.TrimSpace(language) != "" {
		args = append(args, "-l", strings.TrimSpace(language))
	}
	if c.config.Threads > 0 {
		args = append(args, "-t", strconv.Itoa(c.config.Threads))
	}
	return args
}

func defaultFFMpegArgs() []string {
	return []string{"-y", "-i", "{{input}}", "-ar", "16000", "-ac", "1", "{{wav}}"}
}

func defaultWhisperArgs() []string {
	return []string{"-m", "{{model}}", "-f", "{{wav}}", "--no-timestamps"}
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
	runtime := filepath.Join(c.runtime.RuntimeComponentHomePath(), filepath.Base(host))
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

func cleanTranscript(text string) string {
	return strings.TrimSpace(text)
}

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return commandDefinitions()
}
