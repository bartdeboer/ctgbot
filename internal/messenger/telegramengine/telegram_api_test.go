package telegramengine

import (
	"testing"

	"github.com/go-telegram/bot/models"
)

func TestTelegramMessageTextUsesCaptionFallback(t *testing.T) {
	t.Parallel()

	msg := &models.Message{Caption: "please review"}
	if got := telegramMessageText(msg); got != "please review" {
		t.Fatalf("telegramMessageText() = %q, want %q", got, "please review")
	}
}

func TestTelegramMessageAttachmentsCollectsCommonMedia(t *testing.T) {
	t.Parallel()

	msg := &models.Message{
		ID: 55,
		Document: &models.Document{
			FileID:   "doc-1",
			FileName: "report.pdf",
			MimeType: "application/pdf",
		},
		Photo: []models.PhotoSize{
			{FileID: "photo-small", Width: 100, Height: 100},
			{FileID: "photo-large", Width: 200, Height: 200},
		},
		Voice: &models.Voice{
			FileID:   "voice-1",
			MimeType: "audio/ogg",
		},
	}

	attachments := telegramMessageAttachments(msg)
	if len(attachments) != 3 {
		t.Fatalf("len(attachments) = %d, want 3", len(attachments))
	}
	if attachments[0].Kind != "document" || attachments[0].Filename != "report.pdf" {
		t.Fatalf("unexpected document attachment: %+v", attachments[0])
	}
	if attachments[1].Kind != "photo" || attachments[1].FileID != "photo-large" || attachments[1].Filename != "photo-55.jpg" {
		t.Fatalf("unexpected photo attachment: %+v", attachments[1])
	}
	if attachments[2].Kind != "voice" || attachments[2].Filename != "voice-55.ogg" {
		t.Fatalf("unexpected voice attachment: %+v", attachments[2])
	}
}
