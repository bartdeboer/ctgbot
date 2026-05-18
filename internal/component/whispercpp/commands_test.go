package whispercpp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/bartdeboer/go-clir"
)

func TestBuildTranscribeCommandReadsAudioAndFlags(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "voice.ogg")
	if err := os.WriteFile(path, []byte("audio bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	built, err := buildTranscribeCommand(&clir.Request{
		Params: map[string]string{"path": path},
		Extra:  []string{"--type", "audio/ogg", "--model", "large-v3-turbo", "--language", "nl"},
	})
	if err != nil {
		t.Fatalf("buildTranscribeCommand() error = %v", err)
	}
	cmd := built.(transcribeCommand)
	if string(cmd.Media.Content) != "audio bytes" {
		t.Fatalf("content = %q", string(cmd.Media.Content))
	}
	if cmd.Media.Filename != "voice.ogg" || cmd.Media.ContentType != "audio/ogg" {
		t.Fatalf("media = %#v", cmd.Media)
	}
	if cmd.Model != "large-v3-turbo" || cmd.Language != "nl" {
		t.Fatalf("cmd = %#v", cmd)
	}
}

func TestWhisperArgsUseConfiguredTemplateAndRuntimeValues(t *testing.T) {
	c := &Component{config: ComponentConfig{
		WhisperArgs: []string{"-m", "{{model}}", "-f", "{{wav}}"},
		Language:    "en",
		Threads:     4,
	}}
	args := c.whisperArgs(map[string]string{"model": "/models/model.bin", "wav": "/work/input.wav"}, c.config.Language)
	want := []string{"-m", "/models/model.bin", "-f", "/work/input.wav", "-l", "en", "-t", "4"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", args, want)
		}
	}
}

func TestDefaultWhisperArgsWriteTranscriptFile(t *testing.T) {
	c := &Component{}
	args := c.whisperArgs(map[string]string{
		"model":         "/models/model.bin",
		"wav":           "/work/input.wav",
		"output_prefix": "/work/transcript",
	}, "")
	want := []string{"-m", "/models/model.bin", "-f", "/work/input.wav", "-fa", "-np", "-otxt", "-of", "/work/transcript", "-l", "auto"}
	if len(args) != len(want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", args, want)
		}
	}
}
