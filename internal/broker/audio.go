package broker

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/coremodel"
	"github.com/bartdeboer/ctgbot/internal/languagedetect"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type turnMode string

const (
	turnModeText  turnMode = "text"
	turnModeAudio turnMode = "audio"
)

type turnOptions struct {
	Mode           turnMode
	SpeechLanguage string
}

func (o turnOptions) WantsSpeechReply() bool {
	return o.Mode == turnModeAudio
}

func voiceInputAttachment(text string, attachments []message.Media) (message.Media, bool) {
	if strings.TrimSpace(text) != "" || len(attachments) != 1 {
		return message.Media{}, false
	}
	media := attachments[0]
	if !isAudioMedia(media) {
		return message.Media{}, false
	}
	return media, true
}

func isAudioMedia(media message.Media) bool {
	switch strings.ToLower(strings.TrimSpace(media.Kind)) {
	case "audio", "voice":
		return true
	}
	contentType := strings.ToLower(strings.TrimSpace(media.ContentType))
	if contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err == nil {
			contentType = mediaType
		}
		if strings.HasPrefix(contentType, "audio/") {
			return true
		}
	}
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(media.Filename))) {
	case ".aac", ".flac", ".m4a", ".mp3", ".oga", ".ogg", ".opus", ".wav", ".weba":
		return true
	default:
		return false
	}
}

func transcriberForRuntime(runtime *ChatRuntime) (component.Transcriber, string, error) {
	var transcriber component.Transcriber
	var ref string
	for _, loaded := range runtimeComponents(runtime) {
		candidate, ok := loaded.Component.(component.Transcriber)
		if !ok {
			continue
		}
		if transcriber != nil {
			return nil, "", fmt.Errorf("multiple transcribers configured; bind exactly one transcriber for audio turns")
		}
		transcriber = candidate
		ref = loaded.Registration.Ref()
	}
	return transcriber, ref, nil
}

func synthesizerForRuntime(runtime *ChatRuntime) (component.SpeechSynthesizer, string, error) {
	var synthesizer component.SpeechSynthesizer
	var ref string
	for _, loaded := range runtimeComponents(runtime) {
		candidate, ok := loaded.Component.(component.SpeechSynthesizer)
		if !ok {
			continue
		}
		if synthesizer != nil {
			return nil, "", fmt.Errorf("multiple speech synthesizers configured; bind exactly one synthesizer for audio turns")
		}
		synthesizer = candidate
		ref = loaded.Registration.Ref()
	}
	return synthesizer, ref, nil
}

func runtimeComponents(runtime *ChatRuntime) []*component.Loaded {
	if runtime == nil {
		return nil
	}
	return runtime.Components
}

type transcriptionOutcome struct {
	Text     string
	Ref      string
	Model    string
	Language string
}

func transcribeInboundAudio(ctx context.Context, runtime *ChatRuntime, threadID modeluuid.UUID, media message.Media) (transcriptionOutcome, error) {
	transcriber, ref, err := transcriberForRuntime(runtime)
	if err != nil {
		return transcriptionOutcome{}, err
	}
	if transcriber == nil {
		return transcriptionOutcome{}, fmt.Errorf("audio message received but no transcriber is configured")
	}
	result, err := transcriber.Transcribe(ctx, component.TranscriptionRequest{Media: media, ThreadID: threadID})
	if err != nil {
		return transcriptionOutcome{Ref: ref}, err
	}
	text := strings.TrimSpace(result.Text)
	if text == "" {
		return transcriptionOutcome{Ref: ref}, fmt.Errorf("audio transcription via %s returned empty text", ref)
	}
	return transcriptionOutcome{
		Text:     text,
		Ref:      ref,
		Model:    strings.TrimSpace(result.Model),
		Language: strings.TrimSpace(result.Language),
	}, nil
}

func synthesizeTurnReply(ctx context.Context, runtime *ChatRuntime, threadID modeluuid.UUID, options turnOptions, settings turnSettings, text string) (*message.Media, string, error) {
	synthesizer, ref, err := synthesizerForRuntime(runtime)
	if err != nil || synthesizer == nil {
		return nil, "", err
	}
	result, err := synthesizer.Synthesize(ctx, speechRequestForTurn(text, threadID, options, settings))
	if err != nil {
		return nil, ref, err
	}
	if len(result.Media.Content) == 0 {
		return nil, ref, fmt.Errorf("speech synthesis via %s returned empty media", ref)
	}
	return &result.Media, ref, nil
}

func speechRequestForTurn(text string, threadID modeluuid.UUID, options turnOptions, settings turnSettings) component.SpeechRequest {
	language := cleanLanguageCode(settings.Voice.Language)
	if language == "" {
		language = replySpeechLanguage(text, options.SpeechLanguage)
	}
	return component.SpeechRequest{
		Text:     strings.TrimSpace(text),
		ThreadID: threadID,
		Language: language,
		Voice:    strings.TrimSpace(settings.Voice.Name),
		Model:    strings.TrimSpace(settings.Voice.Model),
	}
}

func replySpeechLanguage(text string, inputLanguage string) string {
	inputLanguage = cleanLanguageCode(inputLanguage)
	if inputLanguage == "" {
		return ""
	}
	candidates := []string{inputLanguage}
	if inputLanguage != "en" {
		candidates = append(candidates, "en")
	}
	if detected, ok := languagedetect.Detect(text, candidates); ok {
		return detected
	}
	return inputLanguage
}

func cleanLanguageCode(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if idx := strings.IndexAny(value, "-_"); idx >= 0 {
		value = value[:idx]
	}
	return value
}

func transcriptionMetadata(media message.Media, result transcriptionOutcome) []string {
	var metadata []string
	metadata = append(metadata, "input=audio")
	if result.Ref != "" {
		metadata = append(metadata, "transcriber="+result.Ref)
	}
	if result.Model != "" {
		metadata = append(metadata, "transcription_model="+result.Model)
	}
	if result.Language != "" {
		metadata = append(metadata, "transcription_language="+result.Language)
	}
	if filename := strings.TrimSpace(media.Filename); filename != "" {
		metadata = append(metadata, "original_filename="+filename)
	}
	if contentType := strings.TrimSpace(media.ContentType); contentType != "" {
		metadata = append(metadata, "original_content_type="+contentType)
	}
	return metadata
}

func (b *Broker) relayVoiceTranscript(ctx context.Context, runtime *ChatRuntime, thread coremodel.Thread, providerMessageID string, transcript string) error {
	if runtime == nil || strings.TrimSpace(transcript) == "" {
		return nil
	}
	return b.relayPayloadToRelayBindings(ctx, runtime.Relays, thread, message.OutboundPayload{
		SupersedesProviderMessageID: strings.TrimSpace(providerMessageID),
		Text:                        message.TextMessage{Text: strings.TrimSpace(transcript)},
	})
}

func (b *Broker) relaySynthesizedTurnReply(ctx context.Context, runtime *ChatRuntime, thread coremodel.Thread, options turnOptions, settings turnSettings, text string) error {
	if runtime == nil || strings.TrimSpace(text) == "" {
		return nil
	}
	media, _, err := synthesizeTurnReply(ctx, runtime, thread.ID, options, settings, text)
	if err != nil || media == nil {
		return err
	}
	return b.relayPayloadToRelayBindings(ctx, runtime.Relays, thread, message.OutboundPayload{
		Attachments: []message.Media{*media},
	})
}
