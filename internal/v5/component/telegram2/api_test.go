package telegram2

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramMessageTextUsesCaptionFallback(t *testing.T) {
	if got := telegramMessageText(&models.Message{Text: " hello ", Caption: "caption"}); got != " hello " {
		t.Fatalf("telegramMessageText() = %q, want text", got)
	}
	if got := telegramMessageText(&models.Message{Caption: " caption "}); got != "caption" {
		t.Fatalf("telegramMessageText() = %q, want trimmed caption", got)
	}
}

func TestTelegramMessageAttachmentsCollectsCommonMedia(t *testing.T) {
	attachments := telegramMessageAttachments(&models.Message{
		ID: 42,
		Document: &models.Document{
			FileID:   "doc-file",
			FileName: "report.md",
			MimeType: "text/markdown",
		},
		Photo:     []models.PhotoSize{{FileID: "small"}, {FileID: "large"}},
		Video:     &models.Video{FileID: "video-file", MimeType: "video/mp4"},
		Audio:     &models.Audio{FileID: "audio-file", FileName: "song.mp3", MimeType: "audio/mpeg"},
		Voice:     &models.Voice{FileID: "voice-file", MimeType: "audio/ogg"},
		Animation: &models.Animation{FileID: "gif-file", MimeType: "image/gif"},
	})

	if len(attachments) != 6 {
		t.Fatalf("len(attachments) = %d, want 6", len(attachments))
	}
	checks := []struct {
		index    int
		kind     string
		fileID   string
		filename string
	}{
		{0, "document", "doc-file", "report.md"},
		{1, "photo", "large", "photo-42.jpg"},
		{2, "video", "video-file", "video-42.mp4"},
		{3, "audio", "audio-file", "song.mp3"},
		{4, "voice", "voice-file", "voice-42.ogg"},
		{5, "animation", "gif-file", "animation-42.gif"},
	}
	for _, check := range checks {
		got := attachments[check.index]
		if got.Kind != check.kind || got.FileID != check.fileID || got.Filename != check.filename {
			t.Fatalf("attachments[%d] = %#v, want kind=%q fileID=%q filename=%q", check.index, got, check.kind, check.fileID, check.filename)
		}
	}
}
