package outlook

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/oauth2"
)

const DefaultCallbackPort = 1465

var oauthScopes = []string{"openid", "profile", "email", "offline_access", "User.Read", "Mail.Read"}
var errMissingAuthMaterial = errors.New("missing outlook auth material")

type oauthClientFile struct {
	ClientID string `json:"client_id"`
	Tenant   string `json:"tenant,omitempty"`
}

func (c *Component) Auth(ctx context.Context, callbackPort int, callbackTimeout time.Duration, stdout io.Writer, stderr io.Writer) error {
	stdout = writerOrDiscard(stdout)
	stderr = writerOrDiscard(stderr)
	oauthConfig, configPath, err := c.loadOAuthConfig()
	if err != nil {
		fmt.Fprintln(stderr, outlookOAuthConfigHelp(c))
		return err
	}
	if callbackPort <= 0 {
		callbackPort = DefaultCallbackPort
	}
	if callbackTimeout <= 0 {
		callbackTimeout = 10 * time.Minute
	}
	listener, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", callbackPort))
	if err != nil {
		return fmt.Errorf("open outlook oauth callback listener: %w", err)
	}
	defer listener.Close()
	port := listener.Addr().(*net.TCPAddr).Port
	oauthConfig.RedirectURL = fmt.Sprintf("http://127.0.0.1:%d/oauth2callback", port)
	state, err := randomState()
	if err != nil {
		return err
	}
	codeCh := make(chan string, 1)
	errCh := make(chan error, 1)
	server := &http.Server{Handler: oauthCallbackHandler(state, codeCh, errCh)}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()
	defer func() { _ = server.Shutdown(context.Background()) }()
	url := oauthConfig.AuthCodeURL(state, oauth2.AccessTypeOffline)
	fmt.Fprintf(stdout, "outlook oauth client: %s\n", configPath)
	fmt.Fprintf(stdout, "open this URL to authenticate %s:\n%s\n", c.registration.Ref(), url)
	waitCtx, cancel := context.WithTimeout(ctx, callbackTimeout)
	defer cancel()
	var code string
	select {
	case <-waitCtx.Done():
		return waitCtx.Err()
	case err := <-errCh:
		return err
	case code = <-codeCh:
	}
	token, err := oauthConfig.Exchange(ctx, code)
	if err != nil {
		return fmt.Errorf("exchange outlook oauth code: %w", err)
	}
	if err := c.saveToken(token); err != nil {
		return err
	}
	client, err := c.clientFromToken(ctx, token)
	if err != nil {
		return err
	}
	me, err := client.Me(ctx)
	if err != nil {
		return fmt.Errorf("check outlook profile: %w", err)
	}
	c.mailboxEmail = strings.TrimSpace(firstNonEmpty(me.Mail, me.UserPrincipalName))
	stateFile, _ := c.loadState()
	stateFile.MailboxEmail = c.mailboxEmail
	_ = c.saveState(stateFile)
	fmt.Fprintf(stdout, "outlook auth completed\naccount: %s\n", firstNonEmpty(c.mailboxEmail, c.userID()))
	return nil
}

func (c *Component) AuthStatus(ctx context.Context, stdout io.Writer, stderr io.Writer) error {
	stdout = writerOrDiscard(stdout)
	_ = stderr
	client, err := c.client(ctx)
	if err != nil {
		fmt.Fprintf(stdout, "outlook auth: not authenticated\n%s\n", outlookOAuthConfigHelp(c))
		return nil
	}
	me, err := client.Me(ctx)
	if err != nil {
		fmt.Fprintf(stdout, "outlook auth: token unavailable\n%v\n", err)
		return nil
	}
	account := strings.TrimSpace(firstNonEmpty(me.Mail, me.UserPrincipalName))
	c.mailboxEmail = account
	fmt.Fprintf(stdout, "outlook auth: authenticated\naccount: %s\n", account)
	return nil
}

func (c *Component) loadOAuthConfig() (*oauth2.Config, string, error) {
	clientID := strings.TrimSpace(c.componentConfig.ClientID)
	tenant := strings.TrimSpace(c.componentConfig.Tenant)
	path := filepath.Join(strings.TrimSpace(c.profile.Path), OAuthClientFilename)
	if data, err := os.ReadFile(path); err == nil {
		var file oauthClientFile
		if err := json.Unmarshal(data, &file); err != nil {
			return nil, path, fmt.Errorf("parse outlook oauth client config %s: %w", path, err)
		}
		if strings.TrimSpace(file.ClientID) != "" {
			clientID = strings.TrimSpace(file.ClientID)
		}
		if strings.TrimSpace(file.Tenant) != "" {
			tenant = strings.TrimSpace(file.Tenant)
		}
	} else if !os.IsNotExist(err) {
		return nil, path, fmt.Errorf("read outlook oauth client config %s: %w", path, err)
	}
	if tenant == "" {
		tenant = DefaultTenant
	}
	if clientID == "" {
		return nil, path, fmt.Errorf("%w: missing outlook client_id", errMissingAuthMaterial)
	}
	base := "https://login.microsoftonline.com/" + tenant + "/oauth2/v2.0"
	return &oauth2.Config{ClientID: clientID, Scopes: oauthScopes, Endpoint: oauth2.Endpoint{AuthURL: base + "/authorize", TokenURL: base + "/token"}}, path, nil
}

func (c *Component) loadToken() (*oauth2.Token, error) {
	data, err := os.ReadFile(filepath.Join(c.profile.Path, TokenFilename))
	if err != nil {
		return nil, err
	}
	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}
	return &token, nil
}
func (c *Component) saveToken(token *oauth2.Token) error {
	path := filepath.Join(c.profile.Path, TokenFilename)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}
func (c *Component) clientFromToken(ctx context.Context, token *oauth2.Token) (outlookClient, error) {
	cfg, _, err := c.loadOAuthConfig()
	if err != nil {
		return nil, err
	}
	return newGraphClient(cfg.Client(ctx, token)), nil
}
func (c *Component) client(ctx context.Context) (outlookClient, error) {
	if c.clientOverride != nil {
		return c.clientOverride, nil
	}
	token, err := c.loadToken()
	if err != nil {
		return nil, fmt.Errorf("%w: outlook is not authenticated", errMissingAuthMaterial)
	}
	return c.clientFromToken(ctx, token)
}

func outlookOAuthConfigHelp(c *Component) string {
	return "outlook auth material missing; install oauth_client.json with {\"client_id\":\"...\",\"tenant\":\"organizations\"} or set component config client_id"
}
func writerOrDiscard(w io.Writer) io.Writer {
	if w == nil {
		return io.Discard
	}
	return w
}
func randomState() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}
func oauthCallbackHandler(wantState string, codeCh chan<- string, errCh chan<- error) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("state") != wantState {
			errCh <- fmt.Errorf("invalid outlook oauth state")
			http.Error(w, "invalid state", 400)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			errCh <- fmt.Errorf("missing outlook oauth code")
			http.Error(w, "missing code", 400)
			return
		}
		fmt.Fprintln(w, "Outlook auth completed. You can close this window.")
		codeCh <- code
	})
}
