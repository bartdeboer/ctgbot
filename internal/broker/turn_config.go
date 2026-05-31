package broker

import (
	"context"
	"fmt"
	"strings"

	turnconfig "github.com/bartdeboer/ctgbot/internal/app/config/turn"
	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/configsurface"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	schemacommands "github.com/bartdeboer/ctgbot/internal/schema/commands"
)

const (
	turnConfigInputVoice        = turnconfig.InputVoice
	turnConfigInputLanguage     = turnconfig.InputLanguage
	turnConfigVoiceOutput       = turnconfig.VoiceOutput
	turnConfigVoiceLanguage     = turnconfig.VoiceLanguage
	turnConfigVoiceName         = turnconfig.VoiceName
	turnConfigVoiceModel        = turnconfig.VoiceModel
	turnConfigVoiceDeviceTarget = turnconfig.VoiceDeviceTarget
)

type turnCommandExecutor struct {
	turn *agentTurnRuntime
	next commandengine.CommandExecutor
}

func (e turnCommandExecutor) ActiveComponents() []string {
	provider, ok := e.next.(interface{ ActiveComponents() []string })
	if !ok || provider == nil {
		return nil
	}
	return provider.ActiveComponents()
}

func (e turnCommandExecutor) Execute(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	switch cmd := req.Command.(type) {
	case schemacommands.TurnInfo:
		_ = cmd
		return e.turn.turnInfo(ctx, req)
	case schemacommands.TurnConfigSet:
		return e.turn.setTurnConfig(ctx, req, cmd.Key, cmd.Value)
	case schemacommands.TurnConfigUnset:
		return e.turn.unsetTurnConfig(ctx, req, cmd.Key)
	case schemacommands.TurnConfigGet:
		return e.turn.getTurnConfig(ctx, req, cmd.Key)
	case schemacommands.TurnConfigList:
		return e.turn.listTurnConfig(ctx, req)
	default:
		if e.next == nil {
			return commandengine.Result{}, fmt.Errorf("missing command executor")
		}
		return e.next.Execute(ctx, req)
	}
}

func (e turnCommandExecutor) Run(ctx context.Context, base commandengine.Request, argv []string) (commandengine.Result, error) {
	parser, ok := e.next.(interface {
		Parse(context.Context, commandengine.Request, []string) (commandengine.Request, error)
	})
	if !ok || parser == nil {
		return commandengine.Result{}, fmt.Errorf("missing command parser")
	}
	req, err := parser.Parse(ctx, base, argv)
	if err != nil {
		return commandengine.Result{}, err
	}
	return e.Execute(ctx, req)
}

func (e turnCommandExecutor) Help(ctx context.Context, base commandengine.Request, scope []string) (commandengine.Result, error) {
	helper, ok := e.next.(commandengine.CommandHelper)
	if !ok || helper == nil {
		return commandengine.Result{}, fmt.Errorf("missing command helper")
	}
	return helper.Help(ctx, base, scope)
}

func (r *agentTurnRuntime) turnInfo(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	_, _ = ctx, req
	if r == nil {
		return commandengine.Result{Text: "Turn info\ninput_files: none"}, nil
	}
	values := r.effectiveTurnConfigValues(ctx)
	lines := []string{"Turn info"}
	lines = append(lines, "Input:")
	lines = append(lines, fmt.Sprintf("voice: %t", values.InputVoice))
	if language := strings.TrimSpace(values.InputLanguage); language != "" {
		lines = append(lines, "detected_language: "+language)
	}
	if len(r.inputFiles) == 0 {
		lines = append(lines, "input_files: none")
	} else {
		lines = append(lines, "Input files:")
		for _, file := range r.inputFiles {
			if strings.TrimSpace(file.Path) == "" {
				continue
			}
			lines = append(lines, "- "+formatTurnInputFile(file))
		}
	}
	lines = append(lines, "Use `hostbridge turn config list` for current-turn output options.")
	return commandengine.Result{Text: strings.Join(lines, "\n")}, nil
}

func formatTurnInputFile(file turnInputFile) string {
	parts := []string{strings.TrimSpace(file.Path)}
	if kind := strings.TrimSpace(file.Kind); kind != "" {
		parts = append(parts, "kind="+kind)
	}
	if contentType := strings.TrimSpace(file.ContentType); contentType != "" {
		parts = append(parts, "type="+contentType)
	}
	if filename := strings.TrimSpace(file.Filename); filename != "" {
		parts = append(parts, "filename="+filename)
	}
	if file.Temporary {
		parts = append(parts, "temporary=true")
	}
	return strings.Join(parts, " ")
}

func (r *agentTurnRuntime) setTurnConfig(ctx context.Context, req commandengine.Request, key string, value string) (commandengine.Result, error) {
	values := r.turnConfigValues()
	surface := turnconfig.NewSurface(&values)
	key = turnconfig.NormalizeKey(key)
	if err := surface.ConfigSet(ctx, req, key, value); err != nil {
		return commandengine.Result{}, err
	}
	r.applyTurnConfigValues(values)
	updated, err := surface.ConfigGet(ctx, req, key)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("%s=%s", key, updated)}, nil
}

func (r *agentTurnRuntime) unsetTurnConfig(ctx context.Context, req commandengine.Request, key string) (commandengine.Result, error) {
	values := r.turnConfigValues()
	surface := turnconfig.NewSurface(&values)
	key = turnconfig.NormalizeKey(key)
	if err := surface.ConfigUnset(ctx, req, key); err != nil {
		return commandengine.Result{}, err
	}
	r.applyTurnConfigValues(values)
	updated, err := surface.ConfigGet(ctx, req, key)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: fmt.Sprintf("%s=%s", key, updated)}, nil
}

func (r *agentTurnRuntime) getTurnConfig(ctx context.Context, req commandengine.Request, key string) (commandengine.Result, error) {
	values := r.effectiveTurnConfigValues(ctx)
	surface := turnconfig.NewSurface(&values)
	key = turnconfig.NormalizeKey(key)
	schema := r.turnConfigSchema(ctx)
	field, ok := schema.Field(key)
	if !ok {
		return commandengine.Result{}, turnconfig.UnknownKey(key)
	}
	value, err := surface.ConfigGet(ctx, req, field.Key)
	if err != nil {
		return commandengine.Result{}, err
	}
	return commandengine.Result{Text: configsurface.FormatGet(field, value)}, nil
}

func (r *agentTurnRuntime) listTurnConfig(ctx context.Context, req commandengine.Request) (commandengine.Result, error) {
	values := r.effectiveTurnConfigValues(ctx)
	surface := turnconfig.NewSurface(&values)
	return commandengine.Result{Text: configsurface.FormatList(ctx, req, surface, r.turnConfigSchema(ctx))}, nil
}

func (r *agentTurnRuntime) turnConfigValues() turnconfig.Values {
	if r == nil {
		return turnconfig.Values{}
	}
	return turnconfig.Values{
		InputVoice:        r.voiceInput,
		InputLanguage:     r.detectedInputLanguage,
		VoiceOutput:       r.voiceOutput,
		VoiceLanguage:     r.voiceLanguage,
		VoiceName:         r.voiceName,
		VoiceModel:        r.voiceModel,
		VoiceDeviceTarget: r.voiceDeviceTarget,
	}
}

func (r *agentTurnRuntime) effectiveTurnConfigValues(ctx context.Context) turnconfig.Values {
	values := r.turnConfigValues()
	if strings.TrimSpace(values.VoiceModel) == "" {
		if registry, ok := modelRegistryForRuntime(r.runtime); ok {
			if name, err := registry.DefaultModelForMode(ctx, component.ModelModeTTS); err == nil {
				values.VoiceModel = strings.TrimSpace(name)
			}
		}
	}
	return values
}

func (r *agentTurnRuntime) turnConfigSchema(ctx context.Context) configsurface.ConfigSchema {
	schema := turnconfig.Schema()
	registry, ok := modelRegistryForRuntime(r.runtime)
	if !ok {
		return schema
	}
	ttsModels, defaultModel := modelOptionsForMode(ctx, registry, component.ModelModeTTS)
	schema = withFieldSchema(schema, turnConfigVoiceModel, func(field configsurface.FieldSchema) configsurface.FieldSchema {
		field.Type = configsurface.FieldTypeEnum
		field.Options = ttsModels
		field.Default = defaultModel
		return field
	})
	selectedModel := strings.TrimSpace(r.voiceModel)
	if selectedModel == "" {
		selectedModel = defaultModel
	}
	if selectedModel == "" {
		return schema
	}
	modelSchema, err := registry.ModelConfigSchema(ctx, selectedModel)
	if err != nil {
		return schema
	}
	schema = withModelFieldMetadata(schema, modelSchema, turnConfigVoiceLanguage)
	schema = withModelFieldMetadata(schema, modelSchema, turnConfigVoiceName)
	return schema
}

func withModelFieldMetadata(schema configsurface.ConfigSchema, modelSchema configsurface.ConfigSchema, key string) configsurface.ConfigSchema {
	// Model config metadata is keyed by the existing turn config keys, e.g.
	// voice.language and voice.name. Keep this coupling explicit and narrow:
	// model cards describe valid values for turn output controls; they do not
	// define new turn config fields.
	modelField, ok := modelSchema.Field(key)
	if !ok {
		return schema
	}
	return withFieldSchema(schema, key, func(field configsurface.FieldSchema) configsurface.FieldSchema {
		if modelField.Help != "" {
			field.Help = modelField.Help
		}
		if modelField.Type != "" {
			field.Type = modelField.Type
		}
		if modelField.Default != "" {
			field.Default = modelField.Default
		}
		if len(modelField.Options) > 0 {
			field.Options = append([]string(nil), modelField.Options...)
		}
		return field
	})
}

func modelRegistryForRuntime(runtime *ChatRuntime) (component.ModelRegistry, bool) {
	for _, loaded := range runtimeComponents(runtime) {
		if loaded == nil {
			continue
		}
		registry, ok := loaded.Component.(component.ModelRegistry)
		if ok {
			return registry, true
		}
	}
	return nil, false
}

func modelOptionsForMode(ctx context.Context, registry component.ModelRegistry, mode component.ModelMode) ([]string, string) {
	if registry == nil {
		return nil, ""
	}
	models, err := registry.ListModels(ctx)
	if err != nil {
		return nil, ""
	}
	var options []string
	for _, model := range models {
		if model.Mode == mode && strings.TrimSpace(model.Name) != "" {
			options = append(options, strings.TrimSpace(model.Name))
		}
	}
	defaultModel, _ := registry.DefaultModelForMode(ctx, mode)
	return options, strings.TrimSpace(defaultModel)
}

func withFieldSchema(schema configsurface.ConfigSchema, key string, update func(configsurface.FieldSchema) configsurface.FieldSchema) configsurface.ConfigSchema {
	key = configsurface.NormalizeKey(key)
	for i, field := range schema.Fields {
		if configsurface.NormalizeKey(field.Key) == key {
			schema.Fields[i] = update(field)
			return schema
		}
	}
	return schema
}

func (r *agentTurnRuntime) applyTurnConfigValues(values turnconfig.Values) {
	if r == nil {
		return
	}
	r.voiceOutput = values.VoiceOutput
	r.voiceLanguage = values.VoiceLanguage
	r.voiceName = values.VoiceName
	r.voiceModel = values.VoiceModel
	r.voiceDeviceTarget = values.VoiceDeviceTarget
}

func (r *agentTurnRuntime) turnConfigValue(key string) string {
	value, _ := turnconfig.Value(r.turnConfigValues(), key)
	return value
}

func (r *agentTurnRuntime) applyThreadVoiceConfig(thread coremodel.Thread) {
	if r == nil {
		return
	}
	if thread.VoiceOutput || (r.voiceInput && thread.VoiceReplyToVoiceInput) {
		r.voiceOutput = true
	}
	if language := cleanLanguageCode(thread.VoiceLanguage); language != "" {
		r.voiceLanguage = language
	}
	if voice := strings.TrimSpace(thread.VoiceName); voice != "" {
		r.voiceName = voice
	}
	if model := strings.TrimSpace(thread.VoiceModel); model != "" {
		r.voiceModel = model
	}
	if target := strings.TrimSpace(thread.VoiceDeviceTarget); target != "" {
		r.voiceDeviceTarget = target
	}
}
