package supertonic

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
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
	runtimeimage "github.com/bartdeboer/ctgbot/internal/runtime/image"
	"github.com/bartdeboer/ctgbot/internal/sandboxengine"
	"github.com/bartdeboer/ctgbot/internal/workgate"
)

type ComponentResolver interface {
	ResolveComponentRef(ctx context.Context, ref string) (*coremodel.Component, error)
	ResolveComponent(ctx context.Context, id modeluuid.UUID) (*component.Loaded, error)
}

type Component struct {
	registration      coremodel.Component
	sandboxes         sandboxengine.RuntimeManager
	home              runtimepkg.Home
	runtimeConfig     runtimepkg.BindConfig
	runtimeHome       string
	config            ComponentConfig
	resolver          ComponentResolver
	chatPayloadSender component.ChatPayloadSender
	synthesisGate     *workgate.Gate
	runtimeImage      string
	runtimeDockerfile string
}

var _ component.Component = (*Component)(nil)
var _ component.ProfileOwner = (*Component)(nil)
var _ component.CommandSurface = (*Component)(nil)
var _ component.LocalCommandSurface = (*Component)(nil)
var _ component.SpeechSynthesizer = (*Component)(nil)
var _ component.RuntimeImageProvider = (*Component)(nil)
var _ component.ChatPayloadSenderReceiver = (*Component)(nil)

func New(ctx context.Context, registration coremodel.Component, runtime runtimepkg.Factory, home runtimepkg.Home, storage repository.Storage, resolver ComponentResolver) (component.Component, error) {
	_, _ = ctx, storage
	runtimeConfig, err := loadRuntimeConfig(home.Path)
	if err != nil {
		return nil, err
	}
	provider, ok := runtime.(sandboxengine.Provider)
	if !ok || provider.SandboxManager() == nil {
		return nil, fmt.Errorf("supertonic requires a sandbox-capable runtime")
	}
	config, err := loadComponentConfig(home.Path)
	if err != nil {
		return nil, err
	}
	return &Component{
		registration:      registration,
		sandboxes:         provider.SandboxManager(),
		home:              home,
		runtimeConfig:     runtimeConfig,
		runtimeHome:       runtime.RuntimeComponentHomePath(registration, home),
		config:            config,
		resolver:          resolver,
		synthesisGate:     workgate.New(),
		runtimeImage:      runtimeConfig.Image,
		runtimeDockerfile: DefaultDockerfile,
	}, nil
}

func (c *Component) Type() string { return Type }

func (c *Component) SetChatPayloadSender(sender component.ChatPayloadSender) {
	if c != nil {
		c.chatPayloadSender = sender
	}
}

func (c *Component) ManagedFiles() []component.ManagedFile {
	return []component.ManagedFile{
		{RelativePath: ComponentConfigFilename, Required: false, Sensitive: false},
		{RelativePath: runtimepkg.ConfigFilename, Required: false, Sensitive: false},
	}
}

func (c *Component) RuntimeImageTargets(ctx context.Context) ([]runtimeimage.Target, error) {
	_ = ctx
	if c == nil {
		return nil, nil
	}
	return []runtimeimage.Target{{
		Name:       Type,
		Ref:        c.registration.Ref(),
		Image:      firstNonEmpty(c.runtimeImage, DefaultImage),
		Dockerfile: firstNonEmpty(c.runtimeDockerfile, DefaultDockerfile),
	}}, nil
}

func (c *Component) Synthesize(ctx context.Context, req component.SpeechRequest) (component.SpeechResult, error) {
	if c == nil || c.sandboxes == nil {
		return component.SpeechResult{}, fmt.Errorf("missing supertonic runtime")
	}
	text := strings.TrimSpace(req.Text)
	if text == "" {
		return component.SpeechResult{}, fmt.Errorf("missing speech text")
	}
	model, err := c.resolveModel(ctx, firstNonEmpty(req.Model, c.config.DefaultModel))
	if err != nil {
		return component.SpeechResult{}, err
	}
	if strings.TrimSpace(model.Path) == "" {
		return component.SpeechResult{}, fmt.Errorf("missing supertonic model path")
	}
	if model.Mode != "" && model.Mode != component.ModelModeTTS {
		return component.SpeechResult{}, fmt.Errorf("model %s is mode=%s, want tts", model.Name, model.Mode)
	}
	release, err := c.acquireSynthesis(ctx, model.Name)
	if err != nil {
		return component.SpeechResult{}, err
	}
	defer release()

	work, cleanup, err := c.prepareWorkdir("synthesize-*")
	if err != nil {
		return component.SpeechResult{}, err
	}
	defer cleanup()
	textHost := filepath.Join(work.host, "input.txt")
	opusHost := filepath.Join(work.host, "speech.ogg")
	metadataHost := filepath.Join(work.host, "metadata.json")
	scriptHost := filepath.Join(work.host, "synthesize.py")
	if err := os.WriteFile(textHost, []byte(text), 0o600); err != nil {
		return component.SpeechResult{}, err
	}
	if err := os.WriteFile(scriptHost, []byte(synthesisScript), 0o700); err != nil {
		return component.SpeechResult{}, err
	}

	hostModelPath := modelHostPath(model.Path)
	workspacePath := filepath.Dir(hostModelPath)
	modelPath := filepath.Join(workspaceRuntimePath, filepath.Base(hostModelPath))
	voice := supertonicVoiceName(firstNonEmpty(req.Voice, c.config.DefaultVoice))
	args := []string{
		filepath.Join(work.runtime, "synthesize.py"),
		"--model-dir", modelPath,
		"--voice", voice,
		"--language", firstNonEmpty(req.Language, c.config.Language),
		"--input", filepath.Join(work.runtime, "input.txt"),
		"--output", filepath.Join(work.runtime, "speech.wav"),
		"--opus-output", filepath.Join(work.runtime, "speech.ogg"),
		"--metadata", filepath.Join(work.runtime, "metadata.json"),
	}
	spec, err := c.sandboxSpec(workspacePath)
	if err != nil {
		return component.SpeechResult{}, err
	}
	session, err := c.sandboxes.BeginSession(ctx, spec, c.sessionOptions())
	if err != nil {
		return component.SpeechResult{}, err
	}
	defer session.Close()

	out, err := session.CombinedOutput(ctx, c.config.PythonCommand, args...)
	if err != nil {
		return component.SpeechResult{}, fmt.Errorf("supertonic command: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	content, err := os.ReadFile(opusHost)
	if err != nil {
		return component.SpeechResult{}, fmt.Errorf("read supertonic speech: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	if len(content) == 0 {
		return component.SpeechResult{}, fmt.Errorf("supertonic returned empty audio")
	}
	metadata := readSynthesisMetadata(metadataHost)
	media := message.Media{
		Kind:            "voice",
		Filename:        "speech.ogg",
		ContentType:     "audio/ogg",
		Content:         content,
		DurationSeconds: roundSeconds(metadata.DurationSeconds),
	}
	return component.SpeechResult{
		Media:            media,
		Model:            model.Name,
		Voice:            voice,
		Language:         firstNonEmpty(req.Language, c.config.Language),
		DurationSeconds:  metadata.DurationSeconds,
		SynthesisSeconds: metadata.SynthesisSeconds,
	}, nil
}

func (c *Component) acquireSynthesis(ctx context.Context, modelName string) (func(), error) {
	if c == nil || c.synthesisGate == nil {
		return func() {}, nil
	}
	return c.synthesisGate.Acquire(ctx, strings.TrimSpace(modelName), c.config.MaxConcurrent)
}

func (c *Component) sandboxSpec(modelDir string) (sandboxengine.SandboxSpec, error) {
	securityOpts, err := containerengine.SeccompSecurityOpts(c.runtimeConfig.Seccomp)
	if err != nil {
		return sandboxengine.SandboxSpec{}, err
	}
	name := sandboxengine.SafeName("ctgbot-"+c.registration.Ref()+"-"+workspaceKey(modelDir), "ctgbot-runtime")
	return *sandboxengine.NewBuilder(name).
		Image(firstNonEmpty(c.runtimeConfig.Image, DefaultImage)).
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

func modelHostPath(modelPath string) string {
	modelPath = strings.TrimSpace(modelPath)
	if modelPath == "" {
		return ""
	}
	if filepath.IsAbs(modelPath) {
		return modelPath
	}
	if abs, err := filepath.Abs(modelPath); err == nil {
		return abs
	}
	return modelPath
}

type synthesisMetadata struct {
	DurationSeconds  float64 `json:"duration_seconds,omitempty"`
	SynthesisSeconds float64 `json:"synthesis_seconds,omitempty"`
	SampleRate       int     `json:"sample_rate,omitempty"`
}

func readSynthesisMetadata(path string) synthesisMetadata {
	data, err := os.ReadFile(path)
	if err != nil {
		return synthesisMetadata{}
	}
	var metadata synthesisMetadata
	_ = json.Unmarshal(data, &metadata)
	return metadata
}

func roundSeconds(value float64) int {
	if value <= 0 {
		return 0
	}
	return int(math.Round(value))
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

const synthesisScript = `
import argparse
import json
import subprocess
import time
from supertonic import TTS

parser = argparse.ArgumentParser()
parser.add_argument("--model-dir", required=True)
parser.add_argument("--voice", required=True)
parser.add_argument("--language", required=True)
parser.add_argument("--input", required=True)
parser.add_argument("--output", required=True)
parser.add_argument("--opus-output", required=True)
parser.add_argument("--metadata", required=True)
args = parser.parse_args()

with open(args.input, "r", encoding="utf-8") as f:
    text = f.read().strip()

t0 = time.time()
tts = TTS(model_dir=args.model_dir, auto_download=False)
style = tts.get_voice_style(voice_name=args.voice)
t1 = time.time()
result = tts.synthesize(text, voice_style=style, lang=args.language)
synthesis_seconds = time.time() - t1
if isinstance(result, tuple):
    wav = result[0]
    duration = result[1] if len(result) > 1 and isinstance(result[1], (int, float)) else 0
else:
    wav = result
    duration = 0
tts.save_audio(wav, args.output)
subprocess.run([
    "ffmpeg",
    "-hide_banner",
    "-loglevel", "error",
    "-y",
    "-i", args.output,
    "-c:a", "libopus",
    "-b:a", "32k",
    "-vbr", "on",
    args.opus_output,
], check=True)
with open(args.metadata, "w", encoding="utf-8") as f:
    json.dump({
        "duration_seconds": duration,
        "synthesis_seconds": synthesis_seconds,
        "sample_rate": getattr(tts, "sample_rate", 0),
        "load_seconds": t1 - t0,
    }, f)
`

func (c *Component) UsesLocalCommandRoutes() bool { return true }

func (c *Component) CommandDefinitions() []commandengine.Definition {
	return commandDefinitions()
}
