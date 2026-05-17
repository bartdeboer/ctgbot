package broker

import (
	"context"
	"fmt"
	"mime"
	"path/filepath"
	"strings"

	"github.com/bartdeboer/ctgbot/internal/component"
	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/bartdeboer/ctgbot/internal/modeluuid"
)

type turnMode string

const (
	turnModeText  turnMode = "text"
	turnModeAudio turnMode = "audio"
)

type turnOptions struct {
	Mode turnMode
}

func audioAttachment(attachments []message.Media) (message.Media, bool) {
	for _, media := range attachments {
		if isAudioMedia(media) {
			return media, true
		}
	}
	return message.Media{}, false
}

func isAudioMedia(media message.Media) bool {
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

func transcribeInboundAudio(ctx context.Context, runtime *ChatRuntime, threadID modeluuid.UUID, media message.Media) (string, string, error) {
	transcriber, ref, err := transcriberForRuntime(runtime)
	if err != nil || transcriber == nil {
		return "", "", err
	}
	result, err := transcriber.Transcribe(ctx, component.TranscriptionRequest{Media: media, ThreadID: threadID})
	if err != nil {
		return "", ref, err
	}
	text := strings.TrimSpace(result.Text)
	if text == "" {
		return "", ref, fmt.Errorf("audio transcription via %s returned empty text", ref)
	}
	return text, ref, nil
}

func synthesizeTurnReply(ctx context.Context, runtime *ChatRuntime, text string) (*message.Media, string, error) {
	synthesizer, ref, err := synthesizerForRuntime(runtime)
	if err != nil || synthesizer == nil {
		return nil, "", err
	}
	result, err := synthesizer.Synthesize(ctx, component.SpeechRequest{Text: text})
	if err != nil {
		return nil, ref, err
	}
	if len(result.Media.Content) == 0 {
		return nil, ref, fmt.Errorf("speech synthesis via %s returned empty media", ref)
	}
	return &result.Media, ref, nil
}

func transcribedAudioPrompt(originalText string, transcript string) string {
	originalText = strings.TrimSpace(originalText)
	transcript = strings.TrimSpace(transcript)
	if transcript == "" {
		return originalText
	}
	if originalText == "" {
		return "Transcribed audio message:\n\n" + transcript
	}
	return originalText + "\n\nTranscribed audio attachment:\n\n" + transcript
}
