package gmail

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bartdeboer/ctgbot/internal/coremodel"
	runtimepkg "github.com/bartdeboer/ctgbot/internal/runtime"
	"golang.org/x/oauth2"
)

func TestAuthStatusMissingTokenMentionsTokenNotOAuthClient(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, OAuthClientFilename), []byte(`{"installed":{"client_id":"id","client_secret":"secret","redirect_uris":["http://127.0.0.1"]}}`), 0o600); err != nil {
		t.Fatalf("WriteFile(oauth_client) error = %v", err)
	}
	component := &Component{
		registration: coremodel.Component{Type: Type, Name: "work"},
		home:         runtimepkg.Home{Path: home},
	}

	var stdout bytes.Buffer
	if err := component.AuthStatus(context.Background(), &stdout, nil); err != nil {
		t.Fatalf("AuthStatus() error = %v", err)
	}
	text := stdout.String()
	for _, want := range []string{
		"gmail auth: not authenticated",
		"token.json missing",
		"ctgbot component gmail/work auth",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("AuthStatus output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "oauth client config missing") {
		t.Fatalf("AuthStatus output claimed oauth client missing despite present file:\n%s", text)
	}
}

func TestAuthStatusMissingOAuthClientMentionsOAuthClient(t *testing.T) {
	home := t.TempDir()
	component := &Component{
		registration: coremodel.Component{Type: Type, Name: "work"},
		home:         runtimepkg.Home{Path: home},
	}
	if err := component.saveToken(&oauth2.Token{AccessToken: "token", TokenType: "Bearer"}); err != nil {
		t.Fatalf("saveToken() error = %v", err)
	}

	var stdout bytes.Buffer
	if err := component.AuthStatus(context.Background(), &stdout, nil); err != nil {
		t.Fatalf("AuthStatus() error = %v", err)
	}
	text := stdout.String()
	if !strings.Contains(text, "gmail oauth client config missing") {
		t.Fatalf("AuthStatus output missing oauth help:\n%s", text)
	}
	if strings.Contains(text, "token.json missing") {
		t.Fatalf("AuthStatus output claimed token missing despite present token:\n%s", text)
	}
}
