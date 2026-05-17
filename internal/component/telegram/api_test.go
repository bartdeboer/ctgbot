package telegram

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/message"
	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

type captureHTTPClient struct {
	t      *testing.T
	fields map[string]string
}

func (c *captureHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if !strings.HasSuffix(req.URL.Path, "/sendVideo") {
		return nil, fmt.Errorf("unexpected path %s", req.URL.Path)
	}
	if err := req.ParseMultipartForm(32 << 20); err != nil {
		return nil, err
	}
	for key, want := range c.fields {
		if got := req.FormValue(key); got != want {
			c.t.Fatalf("form field %s = %q, want %q", key, got, want)
		}
	}
	body := `{"ok":true,"result":{"message_id":1,"date":1,"chat":{"id":123,"type":"private"}}}`
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
	}, nil
}

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

func TestAPISendVideoSetsTelegramMetadataParams(t *testing.T) {
	client := &captureHTTPClient{
		t: t,
		fields: map[string]string{
			"chat_id":            "123",
			"message_thread_id":  "4",
			"caption":            "caption",
			"width":              "1280",
			"height":             "720",
			"duration":           "82",
			"supports_streaming": "true",
		},
	}
	b, err := bot.New("token", bot.WithSkipGetMe(), bot.WithHTTPClient(time.Second, client))
	if err != nil {
		t.Fatalf("bot.New() error = %v", err)
	}
	api := &TelegramAPIV2{token: "token"}
	api.setBot(b)

	err = api.SendVideo(context.Background(), 123, 4, "caption", message.Media{
		Filename:          "video.mp4",
		ContentType:       "video/mp4",
		Content:           []byte("mp4"),
		Width:             1280,
		Height:            720,
		DurationSeconds:   82,
		SupportsStreaming: true,
		Thumbnail: &message.MediaThumbnail{
			Filename:    "thumb.jpg",
			ContentType: "image/jpeg",
			Content:     []byte("jpg"),
		},
	})
	if err != nil {
		t.Fatalf("SendVideo() error = %v", err)
	}
}
