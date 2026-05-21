package commands

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/bartdeboer/ctgbot/internal/commandengine"
	"github.com/bartdeboer/ctgbot/internal/simplerbac"
)

func TestHostbridgeRunDoesNotReadStdin(t *testing.T) {
	originalStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	t.Cleanup(func() {
		os.Stdin = originalStdin
		_ = reader.Close()
		_ = writer.Close()
	})
	os.Stdin = reader

	router, err := commandengine.NewRouter(HostbridgeCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	done := make(chan commandengine.Request, 1)
	errs := make(chan error, 1)
	go func() {
		req, err := router.Parse(context.Background(), commandengine.Request{
			Context: commandengine.Context{
				Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
			},
		}, []string{"run", "echo", "hello"})
		if err != nil {
			errs <- err
			return
		}
		done <- req
	}()

	select {
	case err := <-errs:
		t.Fatalf("Parse() error = %v", err)
	case req := <-done:
		cmd, ok := req.Command.(RunCommand)
		if !ok {
			t.Fatalf("command = %T, want RunCommand", req.Command)
		}
		if cmd.Command != "echo" || !reflect.DeepEqual(cmd.Args, []string{"hello"}) || len(cmd.Stdin) != 0 {
			t.Fatalf("command = %#v, want run echo without stdin", cmd)
		}
	case <-time.After(200 * time.Millisecond):
		_ = writer.Close()
		t.Fatal("Parse() blocked reading stdin")
	}
}

func TestHostbridgeSendFileParsesMediaCommand(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	router, err := commandengine.NewRouter(HostbridgeCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	req, err := router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		},
	}, []string{"sendfile", path, "-caption", "a note", "-syntax", "markdown"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	cmd, ok := req.Command.(SendMedia)
	if !ok {
		t.Fatalf("command = %T, want SendMedia", req.Command)
	}
	if cmd.Filename != "note.txt" || cmd.Caption != "a note" || cmd.ContentType != "text/plain" || cmd.Syntax != "markdown" || string(cmd.Content) != "hello" {
		t.Fatalf("command = %#v, want parsed send media", cmd)
	}
}

func TestHostbridgeSendFileParsesMediaAttributes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "video.mp4")
	thumb := filepath.Join(dir, "thumb.jpg")
	if err := os.WriteFile(path, []byte("mp4"), 0o644); err != nil {
		t.Fatalf("write video fixture: %v", err)
	}
	if err := os.WriteFile(thumb, []byte("jpg"), 0o644); err != nil {
		t.Fatalf("write thumbnail fixture: %v", err)
	}
	router, err := commandengine.NewRouter(HostbridgeCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	req, err := router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		},
	}, []string{"sendfile", path, "--type", "video/mp4", "--width", "1280", "--height", "720", "--duration", "82", "--supports-streaming", "--thumbnail", thumb})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	cmd, ok := req.Command.(SendMedia)
	if !ok {
		t.Fatalf("command = %T, want SendMedia", req.Command)
	}
	if cmd.Width != 1280 || cmd.Height != 720 || cmd.DurationSeconds != 82 || !cmd.SupportsStreaming {
		t.Fatalf("media attributes = %#v, want parsed dimensions/duration/streaming", cmd)
	}
	if cmd.Thumbnail == nil || cmd.Thumbnail.Filename != "thumb.jpg" || string(cmd.Thumbnail.Content) != "jpg" {
		t.Fatalf("thumbnail = %#v, want parsed thumbnail", cmd.Thumbnail)
	}
}

func TestHostbridgeMessageParsesPayloadWithAttachments(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "one.txt")
	second := filepath.Join(dir, "two.bin")
	if err := os.WriteFile(first, []byte("one"), 0o644); err != nil {
		t.Fatalf("write first fixture: %v", err)
	}
	if err := os.WriteFile(second, []byte("two"), 0o644); err != nil {
		t.Fatalf("write second fixture: %v", err)
	}
	router, err := commandengine.NewRouter(HostbridgeCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	req, err := router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		},
	}, []string{"message", "hello", "--type", "text/html", "--syntax", "html", "--attach", first + ";type=text/plain;syntax=txt;name=one-renamed.txt", "--attach", second})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	cmd, ok := req.Command.(SendPayload)
	if !ok {
		t.Fatalf("command = %T, want SendPayload", req.Command)
	}
	if got, want := cmd.Payload.Text.Text, "hello"; got != want {
		t.Fatalf("text = %q, want %q", got, want)
	}
	if got, want := cmd.Payload.Text.ContentType, "text/html"; got != want {
		t.Fatalf("text content type = %q, want %q", got, want)
	}
	if got, want := cmd.Payload.Text.Syntax, "html"; got != want {
		t.Fatalf("text syntax = %q, want %q", got, want)
	}
	if got, want := len(cmd.Payload.Attachments), 2; got != want {
		t.Fatalf("attachments len = %d, want %d", got, want)
	}
	if got, want := cmd.Payload.Attachments[0].Filename, "one-renamed.txt"; got != want {
		t.Fatalf("first filename = %q, want %q", got, want)
	}
	if got, want := cmd.Payload.Attachments[0].ContentType, "text/plain"; got != want {
		t.Fatalf("first content type = %q, want %q", got, want)
	}
	if got, want := cmd.Payload.Attachments[0].Syntax, "txt"; got != want {
		t.Fatalf("first syntax = %q, want %q", got, want)
	}
	if got, want := string(cmd.Payload.Attachments[0].Content), "one"; got != want {
		t.Fatalf("first content = %q, want %q", got, want)
	}
	if got, want := cmd.Payload.Attachments[1].Filename, "two.bin"; got != want {
		t.Fatalf("second filename = %q, want %q", got, want)
	}
}

func TestHostbridgeMessageParsesAttachmentMediaAttributes(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "video.mp4")
	thumb := filepath.Join(dir, "thumb.jpg")
	if err := os.WriteFile(video, []byte("mp4"), 0o644); err != nil {
		t.Fatalf("write video fixture: %v", err)
	}
	if err := os.WriteFile(thumb, []byte("jpg"), 0o644); err != nil {
		t.Fatalf("write thumbnail fixture: %v", err)
	}
	router, err := commandengine.NewRouter(HostbridgeCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	req, err := router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		},
	}, []string{"message", "hello", "--attach", video + ";type=video/mp4;width=1280;height=720;duration=82;streaming=true;thumbnail=" + thumb})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	cmd, ok := req.Command.(SendPayload)
	if !ok {
		t.Fatalf("command = %T, want SendPayload", req.Command)
	}
	if len(cmd.Payload.Attachments) != 1 {
		t.Fatalf("attachments len = %d, want 1", len(cmd.Payload.Attachments))
	}
	got := cmd.Payload.Attachments[0]
	if got.Width != 1280 || got.Height != 720 || got.DurationSeconds != 82 || !got.SupportsStreaming {
		t.Fatalf("media attributes = %#v, want parsed metadata", got)
	}
	if got.Thumbnail == nil || got.Thumbnail.Filename != "thumb.jpg" || string(got.Thumbnail.Content) != "jpg" {
		t.Fatalf("thumbnail = %#v, want parsed thumbnail", got.Thumbnail)
	}
}

func TestHostbridgeMessageUsesFullExistingPathBeforeParsingAttachmentParameters(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "one;type=text-plain.txt")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	router, err := commandengine.NewRouter(HostbridgeCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	req, err := router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		},
	}, []string{"message", "hello", "--attach", path})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	cmd, ok := req.Command.(SendPayload)
	if !ok {
		t.Fatalf("command = %T, want SendPayload", req.Command)
	}
	if got, want := cmd.Payload.Attachments[0].Filename, "one;type=text-plain.txt"; got != want {
		t.Fatalf("filename = %q, want %q", got, want)
	}
	if got := cmd.Payload.Attachments[0].ContentType; got != "" {
		t.Fatalf("content type = %q, want empty", got)
	}
}

func TestHostbridgeMessageRejectsUnknownAttachmentParameter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "one.txt")
	if err := os.WriteFile(path, []byte("one"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	router, err := commandengine.NewRouter(HostbridgeCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}

	_, err = router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		},
	}, []string{"message", "hello", "--attach", path + ";bogus=value"})
	if err == nil || !strings.Contains(err.Error(), "unknown attachment parameter") {
		t.Fatalf("Parse() error = %v, want unknown attachment parameter", err)
	}
}

func TestHostbridgeSendStdinParsesMediaCommand(t *testing.T) {
	originalStdin := os.Stdin
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdin pipe: %v", err)
	}
	t.Cleanup(func() {
		os.Stdin = originalStdin
		_ = reader.Close()
	})
	os.Stdin = reader
	go func() {
		_, _ = io.WriteString(writer, "hello from stdin")
		_ = writer.Close()
	}()

	router, err := commandengine.NewRouter(HostbridgeCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	req, err := router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}},
		},
	}, []string{"sendstdin", "-caption", "a note", "-syntax", "markdown"})
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}
	cmd, ok := req.Command.(SendMedia)
	if !ok {
		t.Fatalf("command = %T, want SendMedia", req.Command)
	}
	if cmd.Filename != "stdin.txt" || cmd.Caption != "a note" || cmd.ContentType != "text/plain" || cmd.Syntax != "markdown" || string(cmd.Content) != "hello from stdin" {
		t.Fatalf("command = %#v, want parsed stdin media", cmd)
	}
}

func TestHostbridgeCommandsRequireAgentRole(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	router, err := commandengine.NewRouter(HostbridgeCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	_, err = router.Parse(context.Background(), commandengine.Request{
		Context: commandengine.Context{
			Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleUser}},
		},
	}, []string{"sendfile", path})
	if err == nil {
		t.Fatal("Parse() as user succeeded, want RBAC denial")
	}
}

func TestHostbridgeTurnCommandsParse(t *testing.T) {
	router, err := commandengine.NewRouter(HostbridgeCommands(), commandengine.SourceHostbridge)
	if err != nil {
		t.Fatalf("NewRouter() error = %v", err)
	}
	base := commandengine.Request{Context: commandengine.Context{Actor: commandengine.Actor{Roles: []simplerbac.Role{simplerbac.RoleAgent}}}}

	infoReq, err := router.Parse(context.Background(), base, []string{"turn", "info"})
	if err != nil {
		t.Fatalf("Parse(turn info) error = %v", err)
	}
	if got, want := infoReq.Command, (TurnInfo{}); !reflect.DeepEqual(got, want) {
		t.Fatalf("turn info command = %#v, want %#v", got, want)
	}

	setReq, err := router.Parse(context.Background(), base, []string{"turn", "config", "set", "voice.language", "nl"})
	if err != nil {
		t.Fatalf("Parse(turn config set) error = %v", err)
	}
	if got, want := setReq.Command, (TurnConfigSet{Key: "voice.language", Value: "nl"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("turn config set command = %#v, want %#v", got, want)
	}

	getReq, err := router.Parse(context.Background(), base, []string{"turn", "config", "get", "voice.name"})
	if err != nil {
		t.Fatalf("Parse(turn config get) error = %v", err)
	}
	if got, want := getReq.Command, (TurnConfigGet{Key: "voice.name"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("turn config get command = %#v, want %#v", got, want)
	}

	listReq, err := router.Parse(context.Background(), base, []string{"turn", "config", "list"})
	if err != nil {
		t.Fatalf("Parse(turn config list) error = %v", err)
	}
	if got, want := listReq.Command, (TurnConfigList{}); !reflect.DeepEqual(got, want) {
		t.Fatalf("turn config list command = %#v, want %#v", got, want)
	}

	unsetReq, err := router.Parse(context.Background(), base, []string{"turn", "config", "unset", "voice.name"})
	if err != nil {
		t.Fatalf("Parse(turn config unset) error = %v", err)
	}
	if got, want := unsetReq.Command, (TurnConfigUnset{Key: "voice.name"}); !reflect.DeepEqual(got, want) {
		t.Fatalf("turn config unset command = %#v, want %#v", got, want)
	}
}
